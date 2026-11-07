package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"dendronotes/internal/config"
	"dendronotes/internal/store"
)

// testIdP is a minimal OpenID Connect provider: discovery, keys, a token
// endpoint that checks PKCE, userinfo and logout.
type testIdP struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu       sync.Mutex
	codes    map[string]idpGrant
	userinfo map[string]any // extra claims served by /userinfo
	issuer   string         // overrides iss in tokens
}

type idpGrant struct {
	challenge string
	nonce     string
	claims    map[string]any
}

func newTestIdP(t *testing.T) *testIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &testIdP{t: t, key: key, codes: map[string]idpGrant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		u := p.srv.URL
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": u, "authorization_endpoint": u + "/auth", "token_endpoint": u + "/token",
			"jwks_uri": u + "/keys", "userinfo_endpoint": u + "/userinfo", "end_session_endpoint": u + "/logout",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
		}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		id, secret, _ := r.BasicAuth()
		if id == "" {
			id, secret = r.FormValue("client_id"), r.FormValue("client_secret")
		}
		p.mu.Lock()
		g, ok := p.codes[r.FormValue("code")]
		delete(p.codes, r.FormValue("code"))
		p.mu.Unlock()
		sum := sha256.Sum256([]byte(r.FormValue("code_verifier")))
		if !ok || id != "dendronotes" || secret != "s3cret" ||
			base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		claims := map[string]any{
			"iss": p.srv.URL, "aud": "dendronotes", "nonce": g.nonce,
			"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
		}
		if p.issuer != "" {
			claims["iss"] = p.issuer
		}
		for k, v := range g.claims {
			claims[k] = v
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-" + g.claims["sub"].(string), "token_type": "Bearer", "expires_in": 300,
			"id_token": p.sign(claims),
		})
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		info := map[string]any{"sub": strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer at-")}
		for k, v := range p.userinfo {
			info[k] = v
		}
		json.NewEncoder(w).Encode(info)
	})
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *testIdP) sign(claims map[string]any) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: p.key, KeyID: "k1"}},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		p.t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	obj, err := signer.Sign(payload)
	if err != nil {
		p.t.Fatal(err)
	}
	out, _ := obj.CompactSerialize()
	return out
}

// grant plays the provider's login page: it accepts the authorization
// request and returns the code for the given claims.
func (p *testIdP) grant(t *testing.T, authURL string, claims map[string]any) (code, state string) {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil || !strings.HasPrefix(authURL, p.srv.URL+"/auth?") {
		t.Fatalf("not sent to the provider: %q", authURL)
	}
	q := u.Query()
	if q.Get("client_id") != "dendronotes" || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" ||
		q.Get("redirect_uri") != "https://notes.example.org/api/auth/oidc/callback" || !strings.Contains(q.Get("scope"), "openid") {
		t.Fatalf("authorization request: %v", q)
	}
	code = "code-" + q.Get("state")[:8]
	p.mu.Lock()
	p.codes[code] = idpGrant{challenge: q.Get("code_challenge"), nonce: q.Get("nonce"), claims: claims}
	p.mu.Unlock()
	return code, q.Get("state")
}

func ssoConfig(t *testing.T, idp *testIdP) config.Config {
	cfg := testConfig(t)
	cfg.Auth.PublicURL = "https://notes.example.org"
	cfg.Auth.OIDC = config.OIDC{
		Issuer: idp.srv.URL, ClientID: "dendronotes", ClientSecret: "s3cret",
		Scopes: []string{"openid", "profile", "email"}, UsernameClaim: "preferred_username", GroupsClaim: "groups",
		AdminGroups: []string{"dn-admins"}, AllowedGroups: []string{"dn-users", "dn-admins"}, LoginLabel: "Keycloak",
	}
	return cfg
}

// ssoLogin runs a whole sign-in and returns the callback response.
func ssoLogin(t *testing.T, s *Server, idp *testIdP, next string, claims map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	c := newClient(t, s)
	rec := c.do("GET", "/api/auth/oidc/login?next="+url.QueryEscape(next), "")
	if rec.Code != http.StatusFound {
		t.Fatalf("login start: %d %s", rec.Code, rec.Body)
	}
	stateCookie := rec.Result().Cookies()[0]
	if stateCookie.SameSite != http.SameSiteLaxMode || !stateCookie.HttpOnly || stateCookie.MaxAge <= 0 {
		t.Errorf("state cookie: %+v", stateCookie)
	}
	code, state := idp.grant(t, rec.Header().Get("Location"), claims)
	c.cookie = stateCookie
	return c.do("GET", "/api/auth/oidc/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), "")
}

// sessionFrom returns a client with the session a callback response set.
func sessionFrom(t *testing.T, s *Server, rec *httptest.ResponseRecorder) *client {
	t.Helper()
	c := newClient(t, s)
	for _, ck := range rec.Result().Cookies() {
		if strings.Contains(ck.Name, "session") && ck.MaxAge > 0 {
			c.cookie = ck
		}
	}
	if c.cookie == nil {
		t.Fatalf("no session cookie: %v", rec.Result().Cookies())
	}
	return c
}

func TestSSOSignIn(t *testing.T) {
	idp := newTestIdP(t)
	s := newTestServerWith(t, testFS(), ssoConfig(t, idp))

	methods := decode[map[string]any](t, newClient(t, s).do("GET", "/api/auth/methods", ""))
	if methods["local"] != true || methods["sso"].(map[string]any)["label"] != "Keycloak" {
		t.Errorf("methods: %v", methods)
	}

	// First sign-in creates the account (with its first soma) and returns to
	// the page the person came from.
	rec := ssoLogin(t, s, idp, "/s/abc/n/Note.md", map[string]any{
		"sub": "u-1", "preferred_username": "Erik H", "name": "Erik", "email": "erik@example.org",
		"groups": []string{"/dn-users"},
	})
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/s/abc/n/Note.md" {
		t.Fatalf("callback: %d %q %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	erik := sessionFrom(t, s, rec)
	me := decode[struct {
		User      store.User
		LogoutURL string
	}](t, erik.do("GET", "/api/auth/me", ""))
	if me.User.Username != "Erik-H" || me.User.Source != store.SourceOIDC || me.User.Role != store.RoleUser ||
		me.User.DisplayName != "Erik" || me.User.Email != "erik@example.org" {
		t.Errorf("account: %+v", me.User)
	}
	if !strings.HasPrefix(me.LogoutURL, idp.srv.URL+"/logout?") || !strings.Contains(me.LogoutURL, "post_logout_redirect_uri=https%3A%2F%2Fnotes.example.org%2F") {
		t.Errorf("logout URL: %q", me.LogoutURL)
	}
	if somas := decode[struct{ Somas []store.Membership }](t, erik.do("GET", "/api/somas", "")).Somas; len(somas) != 1 {
		t.Errorf("first soma: %+v", somas)
	}
	// Name and e-mail come from the provider.
	expect(t, erik.do("PATCH", "/api/me", `{"displayName":"Other"}`), 400, "managed_by_provider")
	expect(t, erik.do("POST", "/api/me/password", `{"current":"x","new":"long enough pw"}`), 400, "not_local")

	// The next sign-in finds the same account, follows the provider's data
	// and groups (admin), and ignores a next URL leading elsewhere.
	rec = ssoLogin(t, s, idp, "https://evil.example/", map[string]any{
		"sub": "u-1", "preferred_username": "erik", "name": "Erik Haeger", "groups": []string{"dn-admins"},
	})
	if rec.Header().Get("Location") != "/" {
		t.Errorf("next: %q", rec.Header().Get("Location"))
	}
	me2 := decode[struct{ User store.User }](t, sessionFrom(t, s, rec).do("GET", "/api/auth/me", "")).User
	if me2.ID != me.User.ID || me2.Role != store.RoleAdmin || me2.DisplayName != "Erik Haeger" || me2.Username != "Erik-H" {
		t.Errorf("second sign-in: %+v", me2)
	}

	// A provider user named like a local account gets an account of its own.
	rec = ssoLogin(t, s, idp, "/", map[string]any{"sub": "u-2", "preferred_username": "admin", "groups": []string{"dn-users"}})
	if u := decode[struct{ User store.User }](t, sessionFrom(t, s, rec).do("GET", "/api/auth/me", "")).User; u.Username != "admin-2" || u.Role != store.RoleUser {
		t.Errorf("name clash: %+v", u)
	}

	// Groups and names may come from userinfo only.
	idp.userinfo = map[string]any{"preferred_username": "dora", "groups": []string{"dn-users"}}
	rec = ssoLogin(t, s, idp, "/", map[string]any{"sub": "u-3"})
	if u := decode[struct{ User store.User }](t, sessionFrom(t, s, rec).do("GET", "/api/auth/me", "")).User; u.Username != "dora" {
		t.Errorf("userinfo: %+v", u)
	}
	idp.userinfo = nil

	audit := decode[struct{ Entries []store.AuditEntry }](t, erik.do("GET", "/api/admin/audit", "")).Entries
	var provisioned int
	for _, e := range audit {
		if e.Action == "user.provision" && e.Detail == "sso" {
			provisioned++
		}
	}
	if provisioned != 3 {
		t.Errorf("provisioned %d accounts, want 3", provisioned)
	}

	srv := decode[map[string]any](t, erik.do("GET", "/api/admin/server", ""))
	if st := srv["sso"].(map[string]any); st["ready"] != true || st["logout"] != true || st["redirectUrl"] != "https://notes.example.org/api/auth/oidc/callback" {
		t.Errorf("sso status: %v", st)
	}
	expect(t, erik.do("POST", "/api/admin/sso/check", ""), 200, "")
}

func TestSSORefusals(t *testing.T) {
	idp := newTestIdP(t)
	s := newTestServerWith(t, testFS(), ssoConfig(t, idp))
	loginError := func(rec *httptest.ResponseRecorder, code string) {
		t.Helper()
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/?login_error="+code {
			t.Errorf("got %d %q, want login_error=%s", rec.Code, rec.Header().Get("Location"), code)
		}
		for _, ck := range rec.Result().Cookies() {
			if strings.Contains(ck.Name, "session") && ck.MaxAge > 0 {
				t.Errorf("session started despite %s", code)
			}
		}
	}

	// Not in an allowed group: no account.
	loginError(ssoLogin(t, s, idp, "/", map[string]any{"sub": "x", "preferred_username": "x", "groups": []string{"guests"}}), "sso_forbidden")
	if _, err := s.store.UserByOIDC(t.Context(), idp.srv.URL, "x"); err == nil {
		t.Error("account created for a refused person")
	}

	// The state must belong to this browser, and works once.
	c := newClient(t, s)
	rec := c.do("GET", "/api/auth/oidc/login", "")
	code, state := idp.grant(t, rec.Header().Get("Location"), map[string]any{"sub": "y", "preferred_username": "y", "groups": []string{"dn-users"}})
	loginError(c.do("GET", "/api/auth/oidc/callback?code="+code+"&state="+state, ""), "sso_failed") // no cookie
	c.cookie = rec.Result().Cookies()[0]
	loginError(c.do("GET", "/api/auth/oidc/callback?code="+code+"&state=forged", ""), "sso_failed")
	if ok := c.do("GET", "/api/auth/oidc/callback?code="+code+"&state="+state, ""); ok.Header().Get("Location") != "/" {
		t.Fatalf("valid callback: %d %q", ok.Code, ok.Header().Get("Location"))
	}
	loginError(c.do("GET", "/api/auth/oidc/callback?code="+code+"&state="+state, ""), "sso_expired")

	// The provider refused, or sent a token that does not fit.
	c = newClient(t, s)
	rec = c.do("GET", "/api/auth/oidc/login", "")
	_, state = idp.grant(t, rec.Header().Get("Location"), nil)
	c.cookie = rec.Result().Cookies()[0]
	loginError(c.do("GET", "/api/auth/oidc/callback?error=access_denied&state="+state, ""), "sso_denied")

	claims := map[string]any{"sub": "z", "preferred_username": "z", "groups": []string{"dn-users"}, "nonce": "not the one"}
	loginError(ssoLogin(t, s, idp, "/", claims), "sso_failed")
	delete(claims, "nonce")
	idp.issuer = "https://other.example"
	loginError(ssoLogin(t, s, idp, "/", claims), "sso_failed")
	idp.issuer = ""

	// Disabled accounts stay out.
	rec = ssoLogin(t, s, idp, "/", claims)
	u, err := s.store.UserByOIDC(t.Context(), idp.srv.URL, "z")
	if err != nil || rec.Code != http.StatusFound {
		t.Fatalf("sign-in: %v %d", err, rec.Code)
	}
	disabled := true
	if _, err := s.store.UpdateUser(t.Context(), u.ID, store.UserUpdate{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	loginError(ssoLogin(t, s, idp, "/", claims), "account_disabled")
}

func TestPasswordsOnlyForAdminNextToSSO(t *testing.T) {
	idp := newTestIdP(t)
	cfg := ssoConfig(t, idp)
	cfg.Auth.LocalLogin = false
	s := newTestServerWith(t, testFS(), cfg)
	admin := newClient(t, s)
	admin.login() // emergency access
	expect(t, admin.do("POST", "/api/admin/users", `{"username":"ben","password":"ben password long","role":"user"}`), 201, "")
	expect(t, newClient(t, s).do("POST", "/api/auth/login", `{"username":"ben","password":"ben password long"}`), 401, "invalid_credentials")
	if m := decode[map[string]any](t, admin.do("GET", "/api/auth/methods", "")); m["local"] != false {
		t.Errorf("methods: %v", m)
	}
}

func TestSSOUnavailable(t *testing.T) {
	idp := newTestIdP(t)
	cfg := ssoConfig(t, idp)
	idp.srv.Close() // provider down
	s := newTestServerWith(t, testFS(), cfg)
	rec := newClient(t, s).do("GET", "/api/auth/oidc/login", "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/?login_error=sso_unavailable" {
		t.Errorf("provider down: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// Without OIDC the endpoints do not exist.
	plain := newTestServer(t, testFS())
	expect(t, newClient(t, plain).do("GET", "/api/auth/oidc/login", ""), 404, "")
	if m := decode[map[string]any](t, newClient(t, plain).do("GET", "/api/auth/methods", "")); m["sso"] != nil || m["local"] != true {
		t.Errorf("methods without OIDC: %v", m)
	}
}
