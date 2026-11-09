// Package auth authenticates requests against the accounts in the store:
// with a user name and password or through an OpenID Connect provider
// (Keycloak), both followed by a session cookie, or via a header set by a
// trusted authenticating proxy (e.g. oauth2-proxy).
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"dendronotes/internal/clientip"
	"dendronotes/internal/httpx"
	"dendronotes/internal/sso"
	"dendronotes/internal/store"
)

// Mode selects how users sign in.
type Mode string

const (
	ModeLocal Mode = "local" // dendronotes signs people in (passwords, OIDC)
	ModeProxy Mode = "proxy" // a trusted proxy names the user in a header
)

const (
	cookieSecure   = "__Host-dendronotes_session" // requires HTTPS, Path=/ and no Domain
	cookieInsecure = "dendronotes_session"
	maxPasswordLen = 1024
	maxUsernameLen = 64
	maxLoginLen    = 254 // user name or e-mail address at sign-in
)

// MinPasswordLength applies to passwords set in the app.
const MinPasswordLength = 10

// Config configures the authenticator.
type Config struct {
	Mode         Mode
	SessionTTL   time.Duration
	CookieSecure string // "auto", "true" or "false"
	HashParams   Params // for new passwords; zero value means DefaultParams

	// AdminUsername is the first admin account (see Bootstrap); in proxy mode
	// the proxy user of that name becomes admin.
	AdminUsername string

	// OnProvision runs after an account was created on first sign-in (proxy,
	// later OIDC), e.g. to give it a first soma. Errors are logged only.
	OnProvision func(context.Context, store.User) error

	// PasswordsAdminOnly turns password sign-in off for everyone but
	// AdminUsername (emergency access next to SSO).
	PasswordsAdminOnly bool

	// SSO signs people in through an OpenID Connect provider (nil: off).
	SSO           *sso.Provider
	SSOLabel      string   // provider name on the sign-in button
	AdminGroups   []string // provider groups whose members are admins (empty: not synced)
	AllowedGroups []string // only members of these groups may sign in (empty: everyone)

	// Directory checks the passwords of accounts that are not local (Active
	// Directory / LDAP, see directory.go); nil: off.
	Directory              Directory
	DirectoryAdminGroups   []string // groups (names or DNs) whose members are admins (empty: not synced)
	DirectoryAllowedGroups []string // only members may sign in (empty: every account found)
	// DirectoryGroups syncs the account's groups below the group base into
	// dendronotes groups; OnDirectoryGroups hears which ones changed members.
	DirectoryGroups   bool
	OnDirectoryGroups func(ctx context.Context, changed []int64)
	// OnLogin counts sign-ins (via: password, directory, sso).
	OnLogin func(via string, ok bool)
	// OnSignedOut runs after the directory sync signed an account out (to
	// end its live streams).
	OnSignedOut func(ctx context.Context, userID int64)

	// Proxy mode.
	ProxyHeader    string
	ProxyAllowed   []string // empty: every user the proxy lets through
	ProxyLogoutURL string
}

// Authenticator checks requests and serves the login endpoints.
type Authenticator struct {
	cfg     Config
	log     *slog.Logger
	ips     *clientip.Resolver
	store   *store.Store
	limiter *rateLimiter // per client IP and globally
	byUser  *rateLimiter // per account name
	// badTokens slows down clients that keep sending invalid API tokens.
	badTokens *rateLimiter
	starts    *rateLimiter // SSO sign-in attempts per client IP
	allowed   map[string]bool
	sem       chan struct{} // bounds concurrent argon2 computations (memory)
	dummy     *Hash         // verified for unknown users, so timing reveals nothing
}

// New validates cfg and builds an authenticator.
func New(cfg Config, st *store.Store, ips *clientip.Resolver, log *slog.Logger) (*Authenticator, error) {
	if cfg.HashParams == (Params{}) {
		cfg.HashParams = DefaultParams
	}
	a := &Authenticator{
		cfg:       cfg,
		log:       log,
		ips:       ips,
		store:     st,
		limiter:   newRateLimiter(12*time.Second, 5, time.Second, 20),
		byUser:    newRateLimiter(30*time.Second, 5, time.Millisecond, 1<<30),
		badTokens: newRateLimiter(2*time.Second, 10, time.Millisecond, 1<<30),
		starts:    newRateLimiter(3*time.Second, 20, 20*time.Millisecond, 500),
		sem:       make(chan struct{}, 2),
	}
	enc, err := HashPassword("not a real password", cfg.HashParams)
	if err != nil {
		return nil, err
	}
	if a.dummy, err = ParseHash(enc); err != nil {
		return nil, err
	}
	switch cfg.Mode {
	case ModeLocal:
	case ModeProxy:
		if cfg.ProxyHeader == "" {
			return nil, errors.New("proxy mode needs a header name")
		}
		if !ips.HasTrusted() {
			return nil, errors.New("proxy mode needs TRUSTED_PROXIES, otherwise anyone could send the user header")
		}
		if len(cfg.ProxyAllowed) > 0 {
			a.allowed = map[string]bool{}
			for _, u := range cfg.ProxyAllowed {
				a.allowed[strings.ToLower(u)] = true
			}
		}
	default:
		return nil, errors.New("unknown auth mode " + strconv.Quote(string(cfg.Mode)))
	}
	return a, nil
}

// Mode returns the configured sign-in mode.
func (a *Authenticator) Mode() Mode { return a.cfg.Mode }

// HashNew hashes a new password with the configured parameters.
func (a *Authenticator) HashNew(password string) (string, error) {
	return HashPassword(password, a.cfg.HashParams)
}

// Bootstrap makes sure an enabled admin account exists. In local mode,
// without one, the account AdminUsername is created (or, if it exists,
// re-enabled as admin with the given password – the way back in after
// losing admin access). hash is a PHC string, password a plain one; one of
// them is needed then. In proxy mode the admin appears with the first request.
func (a *Authenticator) Bootstrap(ctx context.Context, hash, password string) error {
	n, err := a.store.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		if hash != "" || password != "" {
			a.log.Info("an admin account exists; DENDRONOTES_ADMIN_PASSWORD_* is only used to create the first one")
		}
		return nil
	}
	if a.cfg.Mode == ModeProxy {
		a.log.Info("no admin yet: the proxy user becomes admin on first access", "user", a.cfg.AdminUsername)
		return nil
	}
	switch {
	case hash != "":
		if _, err := ParseHash(hash); err != nil {
			return fmt.Errorf("admin password hash: %w", err)
		}
	case password != "":
		if hash, err = a.HashNew(password); err != nil {
			return err
		}
	default:
		return fmt.Errorf("no admin account yet: set DENDRONOTES_ADMIN_PASSWORD_FILE (or _HASH / _HASH_FILE) to create %q", a.cfg.AdminUsername)
	}

	admin, disabled := store.RoleAdmin, false
	u, err := a.store.UserByUsername(ctx, a.cfg.AdminUsername)
	switch {
	case errors.Is(err, store.ErrNotFound):
		u, err = a.store.CreateUser(ctx, store.User{
			Username: a.cfg.AdminUsername, Role: store.RoleAdmin, Source: store.SourceLocal, PasswordHash: hash,
		})
		if err != nil {
			return err
		}
		a.log.Info("admin account created", "user", u.Username)
		a.store.Audit(ctx, store.AuditEntry{Actor: "system", Action: "user.bootstrap", Target: u.Username})
	case err != nil:
		return err
	case u.Source != store.SourceLocal:
		return fmt.Errorf("account %q signs in via %s and cannot become the bootstrap admin", u.Username, u.Source)
	default:
		if _, err := a.store.UpdateUser(ctx, u.ID, store.UserUpdate{Role: &admin, Disabled: &disabled, PasswordHash: &hash}); err != nil {
			return err
		}
		a.log.Warn("no enabled admin: account restored as admin with the configured password", "user", u.Username)
		a.store.Audit(ctx, store.AuditEntry{Actor: "system", Action: "user.restore_admin", Target: u.Username})
	}
	return nil
}

type ctxKey struct{}

type identity struct {
	user      store.User
	sessionID int64
	token     *store.APIToken // signed in with a personal API token
}

// UserFrom returns the account stored by Require.
func UserFrom(ctx context.Context) (store.User, bool) {
	id, ok := ctx.Value(ctxKey{}).(identity)
	return id.user, ok
}

// TokenFrom returns the API token a request was authenticated with, if any.
func TokenFrom(ctx context.Context) (store.APIToken, bool) {
	id, _ := ctx.Value(ctxKey{}).(identity)
	if id.token == nil {
		return store.APIToken{}, false
	}
	return *id.token, true
}

// SessionIDFrom returns the session of the request (0 in proxy mode).
func SessionIDFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(ctxKey{}).(identity)
	return id.sessionID
}

// WithUser returns a context carrying an account (for tests and internal calls).
func WithUser(ctx context.Context, u store.User) context.Context {
	return context.WithValue(ctx, ctxKey{}, identity{user: u})
}

// Require only lets authenticated requests through.
func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.authenticate(r)
		if !ok {
			if _, bearer := bearerToken(r); bearer {
				// Wrong or revoked tokens: scripts retrying in a loop slow down.
				ip := a.ips.ClientIP(r).String()
				if allowed, wait := a.badTokens.allow(ip); !allowed {
					secs := int(math.Ceil(wait.Seconds()))
					w.Header().Set("Retry-After", strconv.Itoa(secs))
					httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retryAfter": secs})
					return
				}
				a.log.Warn("invalid API token", "ip", ip)
			}
			a.unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

// RequireAdmin only lets authenticated admins through.
func (a *Authenticator) RequireAdmin(next http.Handler) http.Handler {
	return a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, _ := UserFrom(r.Context()); u.Role != store.RoleAdmin {
			httpx.Error(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// Authenticated reports whether r still carries a valid session (or a trusted
// proxy identity). Long-lived streams use it to end when the session ends.
func (a *Authenticator) Authenticated(r *http.Request) bool {
	_, ok := a.authenticate(r)
	return ok
}

// bearerToken returns the token of an "Authorization: Bearer …" header.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return "", false
	}
	return strings.TrimSpace(h[7:]), true
}

func (a *Authenticator) authenticate(r *http.Request) (identity, bool) {
	ctx := r.Context()
	// A personal API token decides on its own: no fallback to cookies.
	if token, ok := bearerToken(r); ok {
		u, t, ok := a.store.LookupToken(ctx, token, a.ips.ClientIP(r).String())
		if !ok {
			return identity{}, false
		}
		return identity{user: u, token: &t}, true
	}
	if a.cfg.Mode == ModeProxy {
		u, ok := a.proxyUser(r)
		return identity{user: u}, ok
	}
	for _, name := range []string{cookieSecure, cookieInsecure} {
		if c, err := r.Cookie(name); err == nil {
			if u, sid, ok := a.store.LookupSession(ctx, c.Value, a.cfg.SessionTTL); ok {
				return identity{user: u, sessionID: sid}, true
			}
		}
	}
	return identity{}, false
}

// proxyUser maps the proxy's user header to an account, creating it on first
// sight. Only requests from trusted proxies count.
func (a *Authenticator) proxyUser(r *http.Request) (store.User, bool) {
	if !a.ips.FromTrustedProxy(r) {
		return store.User{}, false
	}
	name := strings.TrimSpace(r.Header.Get(a.cfg.ProxyHeader))
	if name == "" || len(name) > maxUsernameLen || ValidUsername(name) != nil {
		return store.User{}, false
	}
	if a.allowed != nil && !a.allowed[strings.ToLower(name)] {
		a.log.Warn("proxy user not allowed", "user", name)
		return store.User{}, false
	}
	ctx := r.Context()
	u, err := a.store.UserByUsername(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		role := store.RoleUser
		if n, _ := a.store.CountAdmins(ctx); n == 0 && strings.EqualFold(name, a.cfg.AdminUsername) {
			role = store.RoleAdmin
		}
		u, err = a.store.CreateUser(ctx, store.User{Username: name, Role: role, Source: store.SourceProxy})
		if errors.Is(err, store.ErrTaken) {
			u, err = a.store.UserByUsername(ctx, name)
		} else if err == nil {
			a.store.Audit(ctx, store.AuditEntry{ActorID: u.ID, Actor: u.Username, Action: "user.provision", Detail: "proxy", IP: a.ips.ClientIP(r).String()})
			if a.cfg.OnProvision != nil {
				_ = a.cfg.OnProvision(ctx, u)
			}
		}
	}
	if err != nil {
		a.log.Error("proxy account lookup failed", "err", err)
		return store.User{}, false
	}
	if u.Disabled || u.Source != store.SourceProxy {
		return store.User{}, false
	}
	return u, true
}

func (a *Authenticator) unauthorized(w http.ResponseWriter) {
	httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized", "mode": string(a.cfg.Mode)})
}

// MeResponse describes the signed-in account to the frontend.
type MeResponse struct {
	User      store.User `json:"user"`
	Mode      Mode       `json:"mode"`
	LogoutURL string     `json:"logoutUrl,omitempty"`
}

// HandleMe reports who is signed in (401 if nobody).
func (a *Authenticator) HandleMe(w http.ResponseWriter, r *http.Request) {
	id, ok := a.authenticate(r)
	if !ok {
		a.unauthorized(w)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, MeResponse{User: id.user, Mode: a.cfg.Mode, LogoutURL: a.logoutURL(id.user)})
}

// logoutURL is where the browser goes after signing out of dendronotes: the
// proxy's or the OIDC provider's logout, if any.
func (a *Authenticator) logoutURL(u store.User) string {
	switch {
	case a.cfg.Mode == ModeProxy:
		return a.cfg.ProxyLogoutURL
	case u.Source == store.SourceOIDC && a.cfg.SSO != nil:
		return a.cfg.SSO.LogoutURL()
	}
	return ""
}

// MethodsResponse tells the sign-in page which ways to sign in exist.
type MethodsResponse struct {
	Mode  Mode `json:"mode"`
	Local bool `json:"local"` // the password form is offered to everyone
	SSO   *struct {
		Label string `json:"label"`
	} `json:"sso"`
}

// HandleMethods lists the ways to sign in (public).
func (a *Authenticator) HandleMethods(w http.ResponseWriter, r *http.Request) {
	// The password form is for local accounts and for the directory.
	m := MethodsResponse{Mode: a.cfg.Mode, Local: a.cfg.Mode == ModeLocal && (!a.cfg.PasswordsAdminOnly || a.cfg.Directory != nil)}
	if a.cfg.Mode == ModeLocal && a.cfg.SSO != nil {
		m.SSO = &struct {
			Label string `json:"label"`
		}{a.cfg.SSOLabel}
	}
	httpx.WriteJSON(w, http.StatusOK, m)
}

// HandleLogin checks user name (or e-mail address) and password and starts a
// session.
func (a *Authenticator) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Mode != ModeLocal {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	ip := a.ips.ClientIP(r).String()
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	name := strings.TrimSpace(body.Username)
	// The account is looked up before the limits so that its user name and
	// its e-mail address share one budget of attempts.
	var (
		u   store.User
		err = store.ErrNotFound
	)
	if name != "" && len(name) <= maxLoginLen {
		u, err = a.store.UserByLogin(r.Context(), name)
	}
	userKey := "u:" + strings.ToLower(name)
	if err == nil {
		userKey = "u:" + strings.ToLower(u.Username)
	}
	for _, l := range []struct {
		lim *rateLimiter
		key string
	}{{a.limiter, ip}, {a.byUser, userKey}} {
		if ok, wait := l.lim.allow(l.key); !ok {
			secs := int(math.Ceil(wait.Seconds()))
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			a.log.Warn("login rate limited", "ip", ip, "user", name)
			httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retryAfter": secs})
			return
		}
	}

	fail := func(reason string) {
		via := "password"
		if strings.HasSuffix(reason, "(directory)") {
			via = "directory"
		}
		a.counted(via, false)
		a.log.Warn("login failed", "ip", ip, "user", name, "reason", reason)
		a.store.Audit(r.Context(), store.AuditEntry{Actor: name, Action: "login.failed", Detail: reason, IP: ip})
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials")
	}
	if name == "" || len(name) > maxLoginLen || body.Password == "" || len(body.Password) > maxPasswordLen {
		fail("malformed")
		return
	}
	// Not a local password account: the directory decides (if there is one).
	if a.cfg.Directory != nil && (errors.Is(err, store.ErrNotFound) || (err == nil && u.Source == store.SourceLDAP)) {
		a.directoryLogin(w, r, name, body.Password, ip, fail)
		return
	}
	if a.cfg.PasswordsAdminOnly && (err != nil || !strings.EqualFold(u.Username, a.cfg.AdminUsername)) {
		fail("password sign-in is off")
		return
	}

	hash := a.dummy
	if err == nil && u.Source == store.SourceLocal {
		if h, perr := ParseHash(u.PasswordHash); perr == nil {
			hash = h
		}
	}
	select {
	case a.sem <- struct{}{}:
	case <-r.Context().Done():
		return
	}
	ok := hash.Verify(body.Password)
	<-a.sem
	if err != nil || u.Source != store.SourceLocal || hash == a.dummy || !ok {
		fail("credentials")
		return
	}
	a.startSession(w, r, u, ip, "")
}

// counted tells OnLogin about a sign-in.
func (a *Authenticator) counted(via string, ok bool) {
	if a.cfg.OnLogin != nil {
		a.cfg.OnLogin(via, ok)
	}
}

// startSession signs an account in after its password was checked: refused
// for disabled accounts, else a session cookie and the account as answer.
func (a *Authenticator) startSession(w http.ResponseWriter, r *http.Request, u store.User, ip, via string) {
	if via == "" {
		via = "password"
	}
	if u.Disabled {
		a.counted(via, false)
		a.log.Warn("login of disabled account", "ip", ip, "user", u.Username)
		a.store.Audit(r.Context(), store.AuditEntry{ActorID: u.ID, Actor: u.Username, Action: "login.failed", Detail: "disabled", IP: ip})
		httpx.Error(w, http.StatusForbidden, "account_disabled")
		return
	}
	token, err := a.store.CreateSession(r.Context(), u.ID, a.cfg.SessionTTL, ip, r.UserAgent())
	if err != nil {
		a.log.Error("creating session failed", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error")
		return
	}
	_ = a.store.TouchLogin(r.Context(), u.ID)
	a.setCookie(w, r, token)
	a.counted(via, true)
	a.log.Info("login", "ip", ip, "user", u.Username, "via", via)
	a.store.Audit(r.Context(), store.AuditEntry{ActorID: u.ID, Actor: u.Username, Action: "login", Detail: via, IP: ip})
	httpx.WriteJSON(w, http.StatusOK, MeResponse{User: u, Mode: a.cfg.Mode})
}

// HandleLogout ends the session. In proxy mode it returns the URL the
// browser should visit to log out at the proxy.
func (a *Authenticator) HandleLogout(w http.ResponseWriter, r *http.Request) {
	id, _ := a.authenticate(r)
	logout := a.logoutURL(id.user)
	if a.cfg.Mode == ModeLocal {
		for _, name := range []string{cookieSecure, cookieInsecure} {
			if c, err := r.Cookie(name); err == nil {
				if err := a.store.DeleteSession(r.Context(), c.Value); err != nil {
					a.log.Error("deleting session failed", "err", err)
				}
			}
			http.SetCookie(w, &http.Cookie{
				Name: name, Value: "", Path: "/", MaxAge: -1,
				HttpOnly: true, Secure: name == cookieSecure, SameSite: http.SameSiteStrictMode,
			})
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"logoutUrl": logout})
}

func (a *Authenticator) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := a.secure(r)
	name := cookieInsecure
	if secure {
		name = cookieSecure
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		MaxAge:   int(a.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ValidUsername checks a user name: 2–64 bytes of letters, digits and
// . _ - @ (no spaces, nothing that needs escaping in URLs or logs).
func ValidUsername(name string) error {
	if len(name) < 2 || len(name) > maxUsernameLen {
		return errors.New("user names have 2 to 64 characters")
	}
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), strings.ContainsRune("._-@", r):
		default:
			return errors.New("user names may contain letters, digits and . _ - @")
		}
	}
	return nil
}

// CheckPassword verifies a password against an account (for changing it).
func (a *Authenticator) CheckPassword(u store.User, password string) bool {
	h, err := ParseHash(u.PasswordHash)
	if err != nil || len(password) > maxPasswordLen {
		return false
	}
	a.sem <- struct{}{}
	defer func() { <-a.sem }()
	return h.Verify(password)
}

// ValidPassword checks the rules for a new password.
func ValidPassword(pw string) error {
	if len(pw) < MinPasswordLength {
		return fmt.Errorf("passwords have at least %d characters", MinPasswordLength)
	}
	if len(pw) > maxPasswordLen {
		return errors.New("password too long")
	}
	return nil
}
