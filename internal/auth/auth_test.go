package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/clientip"
	"dendronotes/internal/store"
)

// cheap parameters keep the tests fast; production uses DefaultParams.
var cheap = Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func TestHashRoundTrip(t *testing.T) {
	enc, err := HashPassword("correct horse", cheap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("unexpected format: %s", enc)
	}
	h, err := ParseHash(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !h.Verify("correct horse") || h.Verify("correct horsE") || h.Verify("") {
		t.Fatal("verification wrong")
	}
	other, _ := HashPassword("correct horse", cheap)
	if other == enc {
		t.Fatal("salt is not random")
	}
}

func TestParseHashRejects(t *testing.T) {
	good, _ := HashPassword("x", cheap)
	parts := strings.Split(good, "$")
	bad := []string{
		"",
		"plaintext",
		"$argon2i$v=19$m=64,t=1,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=16$m=64,t=1,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=64,t=1,p=1,x=2$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=4194304,t=1,p=1$" + parts[4] + "$" + parts[5], // 4 GiB
		"$argon2id$v=19$m=64,t=99,p=1$" + parts[4] + "$" + parts[5],
		"$argon2id$v=19$m=64,t=1,p=1$c2hvcnQ$" + parts[5], // salt too short
		"$argon2id$v=19$m=64,t=1,p=1$" + parts[4] + "$!!!",
	}
	for _, s := range bad {
		if _, err := ParseHash(s); err == nil {
			t.Errorf("ParseHash(%q) accepted", s)
		}
	}
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(10*time.Second, 3, time.Millisecond, 1000)
	now := time.Now()
	l.now = func() time.Time { return now }
	for i := range 3 {
		if ok, _ := l.allow("1.2.3.4"); !ok {
			t.Fatalf("attempt %d blocked", i+1)
		}
	}
	ok, wait := l.allow("1.2.3.4")
	if ok || wait <= 0 || wait > 10*time.Second {
		t.Fatalf("4th attempt: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.allow("5.6.7.8"); !ok {
		t.Fatal("other client blocked")
	}
	now = now.Add(10 * time.Second)
	if ok, _ := l.allow("1.2.3.4"); !ok {
		t.Fatal("not refilled after waiting")
	}

	global := newRateLimiter(time.Millisecond, 100, time.Hour, 2)
	global.allow("a")
	global.allow("b")
	if ok, _ := global.allow("c"); ok {
		t.Fatal("global limit not enforced across clients")
	}
}

type fixture struct {
	a  *Authenticator
	st *store.Store
}

func newFixture(t *testing.T, mode Mode, trusted ...string) *fixture {
	t.Helper()
	st, err := store.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	var prefixes []netip.Prefix
	for _, p := range trusted {
		prefixes = append(prefixes, netip.MustParsePrefix(p))
	}
	a, err := New(Config{
		Mode: mode, SessionTTL: time.Hour, CookieSecure: "auto", HashParams: cheap,
		AdminUsername: "admin", ProxyHeader: "X-Forwarded-User",
	}, st, clientip.New(prefixes), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{a: a, st: st}
}

func (f *fixture) addUser(t *testing.T, name, password string, role store.Role) store.User {
	t.Helper()
	hash, _ := HashPassword(password, cheap)
	u, err := f.st.CreateUser(context.Background(), store.User{Username: name, Role: role, Source: store.SourceLocal, PasswordHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func login(a *Authenticator, user, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": user, "password": password})
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.HandleLogin(rec, req)
	return rec
}

func whoami(a *Authenticator, cookie *http.Cookie) (int, string) {
	h := a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		w.Write([]byte(u.Username))
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestLoginFlow(t *testing.T) {
	f := newFixture(t, ModeLocal)
	f.addUser(t, "Anna", "correct horse", store.RoleUser)

	for _, c := range [][2]string{{"anna", "wrong"}, {"nobody", "correct horse"}, {"", "x"}} {
		if rec := login(f.a, c[0], c[1]); rec.Code != 401 {
			t.Fatalf("login %v: %d", c, rec.Code)
		}
	}
	rec := login(f.a, "ANNA", "correct horse") // user names ignore case
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	cookie := rec.Result().Cookies()[0]
	if cookie.Name != cookieInsecure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("cookie attributes: %+v", cookie)
	}
	if code, name := whoami(f.a, cookie); code != 200 || name != "Anna" {
		t.Fatalf("authenticated request: %d %q", code, name)
	}
	if code, _ := whoami(f.a, nil); code != 401 {
		t.Fatalf("anonymous request: %d", code)
	}

	// Logout invalidates the token on the server, not just in the browser.
	logoutReq := httptest.NewRequest("POST", "/api/auth/logout", nil)
	logoutReq.AddCookie(cookie)
	f.a.HandleLogout(httptest.NewRecorder(), logoutReq)
	if code, _ := whoami(f.a, cookie); code != 401 {
		t.Fatalf("request after logout: %d", code)
	}

	log, _ := f.st.AuditLog(context.Background(), 0, 10)
	if len(log) != 4 || log[0].Action != "login" || log[1].Action != "login.failed" {
		t.Errorf("audit log: %+v", log)
	}
}

func TestDisabledAccount(t *testing.T) {
	f := newFixture(t, ModeLocal)
	u := f.addUser(t, "anna", "correct horse", store.RoleUser)
	cookie := login(f.a, "anna", "correct horse").Result().Cookies()[0]
	disabled := true
	f.st.UpdateUser(context.Background(), u.ID, store.UserUpdate{Disabled: &disabled})
	if code, _ := whoami(f.a, cookie); code != 401 {
		t.Errorf("session of a disabled account: %d", code)
	}
	if rec := login(f.a, "anna", "correct horse"); rec.Code != 403 {
		t.Errorf("login of a disabled account: %d", rec.Code)
	}
}

func TestSecureCookieBehindTLSProxy(t *testing.T) {
	f := newFixture(t, ModeLocal, "10.0.0.0/8")
	f.addUser(t, "anna", "pw long enough", store.RoleUser)
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"anna","password":"pw long enough"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.RemoteAddr = "10.0.0.2:4000"
	rec := httptest.NewRecorder()
	f.a.HandleLogin(rec, req)
	c := rec.Result().Cookies()[0]
	if c.Name != cookieSecure || !c.Secure {
		t.Fatalf("expected __Host- secure cookie, got %+v", c)
	}
}

func TestLoginRateLimits(t *testing.T) {
	f := newFixture(t, ModeLocal)
	f.addUser(t, "anna", "correct horse", store.RoleUser)
	for range 5 {
		login(f.a, "anna", "wrong")
	}
	rec := login(f.a, "anna", "correct horse")
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("expected 429 with Retry-After, got %d", rec.Code)
	}
	if rec := login(f.a, "anna", "correct horse"+strings.Repeat("x", 2000)); rec.Code == 200 {
		t.Fatal("overlong password accepted")
	}
}

func TestBootstrap(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, ModeLocal)
	if err := f.a.Bootstrap(ctx, "", ""); err == nil || !strings.Contains(err.Error(), "DENDRONOTES_ADMIN_PASSWORD_FILE") {
		t.Fatalf("bootstrap without password: %v", err)
	}
	if err := f.a.Bootstrap(ctx, "", "first admin password"); err != nil {
		t.Fatal(err)
	}
	if rec := login(f.a, "admin", "first admin password"); rec.Code != 200 {
		t.Fatalf("bootstrap admin cannot log in: %d", rec.Code)
	}
	// With an admin in place, the configured password changes nothing.
	if err := f.a.Bootstrap(ctx, "", "another password"); err != nil {
		t.Fatal(err)
	}
	if rec := login(f.a, "admin", "another password"); rec.Code != 401 {
		t.Fatalf("existing admin's password was replaced: %d", rec.Code)
	}

	// No enabled admin left: the account is restored with the new password.
	u, _ := f.st.UserByUsername(ctx, "admin")
	role, disabled := store.RoleUser, true
	f.st.UpdateUser(ctx, u.ID, store.UserUpdate{Role: &role, Disabled: &disabled})
	if err := f.a.Bootstrap(ctx, "", "rescue password long"); err != nil {
		t.Fatal(err)
	}
	if u, _ := f.st.UserByUsername(ctx, "admin"); u.Role != store.RoleAdmin || u.Disabled {
		t.Errorf("admin not restored: %+v", u)
	}
	if rec := login(f.a, "admin", "rescue password long"); rec.Code != 200 {
		t.Fatalf("restored admin cannot log in: %d", rec.Code)
	}
}

func TestRequireAdmin(t *testing.T) {
	f := newFixture(t, ModeLocal)
	f.addUser(t, "boss", "correct horse", store.RoleAdmin)
	f.addUser(t, "anna", "correct horse", store.RoleUser)
	h := f.a.RequireAdmin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	for user, want := range map[string]int{"boss": 200, "anna": 403} {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(login(f.a, user, "correct horse").Result().Cookies()[0])
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", user, rec.Code, want)
		}
	}
}

func TestProxyMode(t *testing.T) {
	if _, err := New(Config{Mode: ModeProxy, ProxyHeader: "X-Forwarded-User"}, nil, clientip.New(nil), slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("proxy mode without trusted proxies accepted")
	}
	f := newFixture(t, ModeProxy, "10.0.0.0/8")
	f.a.allowed = map[string]bool{"admin": true, "erik": true}
	h := f.a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		w.Write([]byte(string(u.Role)))
	}))

	cases := []struct {
		remote, user string
		want         int
		role         string
	}{
		{"10.1.1.1:1", "erik", 200, "user"},
		{"10.1.1.1:1", "admin", 200, "admin"}, // the configured admin name, first admin
		{"203.0.113.5:1", "erik", 401, ""},    // header from an untrusted peer is ignored
		{"10.1.1.1:1", "mallory", 401, ""},    // not in the allow list
		{"10.1.1.1:1", "", 401, ""},
		{"10.1.1.1:1", "bad name", 401, ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = c.remote
		if c.user != "" {
			req.Header.Set("X-Forwarded-User", c.user)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want || (c.want == 200 && rec.Body.String() != c.role) {
			t.Errorf("remote %s user %q: %d %q, want %d %q", c.remote, c.user, rec.Code, rec.Body, c.want, c.role)
		}
	}
	if rec := login(f.a, "erik", "x"); rec.Code != 404 {
		t.Errorf("password login in proxy mode: %d", rec.Code)
	}
}

func TestValidUsername(t *testing.T) {
	for _, ok := range []string{"anna", "erik.h", "ü-ber_1", "a@b"} {
		if err := ValidUsername(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a", "with space", "slash/x", "tab\t", strings.Repeat("x", 65), "<b>"} {
		if ValidUsername(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
