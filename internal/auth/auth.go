// Package auth authenticates requests, either with a single password and a
// session cookie, or via a header set by a trusted authenticating proxy
// (e.g. oauth2-proxy in front of an OIDC provider).
package auth

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dendronotes/internal/clientip"
	"dendronotes/internal/httpx"
)

// Mode selects the authentication method.
type Mode string

const (
	ModePassword Mode = "password"
	ModeProxy    Mode = "proxy"
)

// OwnerUser is the user name in password mode (there is a single user).
const OwnerUser = "owner"

const (
	cookieSecure   = "__Host-dendronotes_session" // requires HTTPS, Path=/ and no Domain
	cookieInsecure = "dendronotes_session"
	maxPasswordLen = 1024
)

// Config configures the authenticator.
type Config struct {
	Mode Mode

	// Password mode: either a PHC argon2id hash or a plaintext password
	// (hashed at startup, kept only in memory).
	PasswordHash string
	Password     string
	HashParams   Params // used for Password; zero value means DefaultParams
	SessionsFile string
	SessionTTL   time.Duration
	CookieSecure string // "auto", "true" or "false"

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
	hash    *Hash
	store   *Store
	limiter *rateLimiter
	allowed map[string]bool
	sem     chan struct{} // bounds concurrent argon2 computations (memory)
}

// New validates cfg and builds an authenticator.
func New(cfg Config, ips *clientip.Resolver, log *slog.Logger) (*Authenticator, error) {
	a := &Authenticator{
		cfg:     cfg,
		log:     log,
		ips:     ips,
		limiter: newRateLimiter(12*time.Second, 5, time.Second, 20),
		sem:     make(chan struct{}, 2),
	}
	switch cfg.Mode {
	case ModePassword:
		switch {
		case cfg.PasswordHash != "":
			h, err := ParseHash(cfg.PasswordHash)
			if err != nil {
				return nil, err
			}
			a.hash = h
		case cfg.Password != "":
			p := cfg.HashParams
			if p == (Params{}) {
				p = DefaultParams
			}
			enc, err := HashPassword(cfg.Password, p)
			if err != nil {
				return nil, err
			}
			if a.hash, err = ParseHash(enc); err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("password mode needs a password hash or password")
		}
		store, err := OpenStore(cfg.SessionsFile, cfg.SessionTTL)
		if err != nil {
			return nil, err
		}
		a.store = store
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
				a.allowed[u] = true
			}
		}
	default:
		return nil, errors.New("unknown auth mode " + strconv.Quote(string(cfg.Mode)))
	}
	return a, nil
}

type ctxKey struct{}

// UserFrom returns the authenticated user stored by Require.
func UserFrom(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(ctxKey{}).(string)
	return u, ok
}

// Require only lets authenticated requests through.
func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := a.authenticate(r)
		if !ok {
			a.unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	})
}

func (a *Authenticator) authenticate(r *http.Request) (string, bool) {
	if a.cfg.Mode == ModeProxy {
		if !a.ips.FromTrustedProxy(r) {
			return "", false
		}
		user := strings.TrimSpace(r.Header.Get(a.cfg.ProxyHeader))
		if user == "" || len(user) > 256 {
			return "", false
		}
		if a.allowed != nil && !a.allowed[user] {
			a.log.Warn("proxy user not allowed", "user", user)
			return "", false
		}
		return user, true
	}
	for _, name := range []string{cookieSecure, cookieInsecure} {
		if c, err := r.Cookie(name); err == nil {
			if sess, ok := a.store.Lookup(c.Value); ok {
				return sess.User, true
			}
		}
	}
	return "", false
}

func (a *Authenticator) unauthorized(w http.ResponseWriter) {
	httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized", "mode": string(a.cfg.Mode)})
}

type meResponse struct {
	User      string `json:"user"`
	Mode      Mode   `json:"mode"`
	LogoutURL string `json:"logoutUrl,omitempty"`
}

// HandleMe reports who is logged in (401 if nobody).
func (a *Authenticator) HandleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := a.authenticate(r)
	if !ok {
		a.unauthorized(w)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, meResponse{User: user, Mode: a.cfg.Mode, LogoutURL: a.cfg.ProxyLogoutURL})
}

// HandleLogin checks the password and starts a session.
func (a *Authenticator) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Mode != ModePassword {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	ip := a.ips.ClientIP(r).String()
	if ok, wait := a.limiter.allow(ip); !ok {
		secs := int(math.Ceil(wait.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		a.log.Warn("login rate limited", "ip", ip)
		httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retryAfter": secs})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if body.Password == "" || len(body.Password) > maxPasswordLen {
		a.log.Warn("login failed", "ip", ip)
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	select {
	case a.sem <- struct{}{}:
	case <-r.Context().Done():
		return
	}
	ok := a.hash.Verify(body.Password)
	<-a.sem

	if !ok {
		a.log.Warn("login failed", "ip", ip)
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	token, err := a.store.Create(OwnerUser)
	if err != nil {
		a.log.Error("creating session failed", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error")
		return
	}
	a.setCookie(w, r, token)
	a.log.Info("login", "ip", ip)
	httpx.WriteJSON(w, http.StatusOK, meResponse{User: OwnerUser, Mode: ModePassword})
}

// HandleLogout ends the session. In proxy mode it returns the URL the
// browser should visit to log out at the proxy.
func (a *Authenticator) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if a.cfg.Mode == ModePassword {
		for _, name := range []string{cookieSecure, cookieInsecure} {
			if c, err := r.Cookie(name); err == nil {
				if err := a.store.Delete(c.Value); err != nil {
					a.log.Error("deleting session failed", "err", err)
				}
			}
			http.SetCookie(w, &http.Cookie{
				Name: name, Value: "", Path: "/", MaxAge: -1,
				HttpOnly: true, Secure: name == cookieSecure, SameSite: http.SameSiteStrictMode,
			})
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"logoutUrl": a.cfg.ProxyLogoutURL})
}

func (a *Authenticator) setCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := a.cfg.CookieSecure == "true" || (a.cfg.CookieSecure != "false" && a.ips.IsHTTPS(r))
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
