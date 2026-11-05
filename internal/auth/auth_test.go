package auth

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/clientip"
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

func TestSessionStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s, err := OpenStore(path, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now() // OpenStore prunes against the real clock
	s.now = func() time.Time { return now }

	token, err := s.Create("owner")
	if err != nil {
		t.Fatal(err)
	}
	if sess, ok := s.Lookup(token); !ok || sess.User != "owner" {
		t.Fatal("fresh session not found")
	}
	if _, ok := s.Lookup(token + "x"); ok {
		t.Fatal("wrong token accepted")
	}

	// The file contains only the token hash, never the token, and is private.
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), token) {
		t.Fatal("token stored in plain text")
	}
	if st, _ := os.Stat(path); st.Mode().Perm()&0o077 != 0 {
		t.Errorf("sessions file is readable by others: %v", st.Mode().Perm())
	}

	// Sliding expiry: use after 90 min extends the session.
	now = now.Add(90 * time.Minute)
	if _, ok := s.Lookup(token); !ok {
		t.Fatal("session expired too early")
	}
	now = now.Add(90 * time.Minute)
	if _, ok := s.Lookup(token); !ok {
		t.Fatal("sliding expiry did not extend the session")
	}

	// Survives a restart.
	now = time.Now()
	if _, ok := s.Lookup(token); !ok { // refresh against the real clock first
		t.Fatal("session lost")
	}
	s2, err := OpenStore(path, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s2.now = s.now
	if _, ok := s2.Lookup(token); !ok {
		t.Fatal("session lost after reopening")
	}

	// Expires after the TTL without use (the sliding refresh above pushed
	// the expiry to about T0+5h).
	now = now.Add(6 * time.Hour)
	if _, ok := s2.Lookup(token); ok {
		t.Fatal("expired session accepted")
	}

	// Delete.
	t2, _ := s2.Create("owner")
	if err := s2.Delete(t2); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Lookup(t2); ok {
		t.Fatal("deleted session accepted")
	}
}

func TestSessionStoreCapsAndCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path, time.Hour)
	if err != nil || s.Len() != 0 {
		t.Fatalf("corrupt file should start empty: %v %d", err, s.Len())
	}
	tick := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { tick = tick.Add(time.Second); return tick }
	first, _ := s.Create("owner")
	for range maxSessions + 5 {
		if _, err := s.Create("owner"); err != nil {
			t.Fatal(err)
		}
	}
	if s.Len() != maxSessions {
		t.Fatalf("store holds %d sessions, cap is %d", s.Len(), maxSessions)
	}
	if _, ok := s.Lookup(first); ok {
		t.Fatal("oldest session was not evicted")
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

func newPasswordAuth(t *testing.T) *Authenticator {
	t.Helper()
	a, err := New(Config{
		Mode:         ModePassword,
		Password:     "correct horse",
		HashParams:   cheap,
		SessionsFile: filepath.Join(t.TempDir(), "sessions.json"),
		SessionTTL:   time.Hour,
		CookieSecure: "auto",
	}, clientip.New(nil), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func login(a *Authenticator, password string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.HandleLogin(rec, req)
	return rec
}

func TestLoginFlow(t *testing.T) {
	a := newPasswordAuth(t)
	protected := a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, _ := UserFrom(r.Context())
		w.Write([]byte(u))
	}))

	if rec := login(a, "wrong"); rec.Code != 401 {
		t.Fatalf("wrong password: %d", rec.Code)
	}
	rec := login(a, "correct horse")
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	cookie := rec.Result().Cookies()[0]
	if cookie.Name != cookieInsecure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("cookie attributes: %+v", cookie)
	}

	req := httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != OwnerUser {
		t.Fatalf("authenticated request: %d %q", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != 401 {
		t.Fatalf("anonymous request: %d", rec.Code)
	}

	// Logout invalidates the token on the server, not just in the browser.
	logoutReq := httptest.NewRequest("POST", "/api/auth/logout", nil)
	logoutReq.AddCookie(cookie)
	a.HandleLogout(httptest.NewRecorder(), logoutReq)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("request after logout: %d", rec.Code)
	}
}

func TestSecureCookieBehindTLSProxy(t *testing.T) {
	trusted, _ := clientip.ParseList("10.0.0.0/8")
	a, err := New(Config{
		Mode: ModePassword, Password: "pw", HashParams: cheap,
		SessionsFile: filepath.Join(t.TempDir(), "s.json"), SessionTTL: time.Hour, CookieSecure: "auto",
	}, clientip.New(trusted), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"pw"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.RemoteAddr = "10.0.0.2:4000"
	rec := httptest.NewRecorder()
	a.HandleLogin(rec, req)
	c := rec.Result().Cookies()[0]
	if c.Name != cookieSecure || !c.Secure {
		t.Fatalf("expected __Host- secure cookie, got %+v", c)
	}
}

func TestLoginRateLimit(t *testing.T) {
	a := newPasswordAuth(t)
	for range 5 {
		login(a, "wrong")
	}
	rec := login(a, "correct horse")
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("expected 429 with Retry-After, got %d", rec.Code)
	}
}

func TestLoginRejectsOverlongPassword(t *testing.T) {
	a := newPasswordAuth(t)
	if rec := login(a, "correct horse"+strings.Repeat("x", 2000)); rec.Code != 401 {
		t.Fatalf("overlong password: %d", rec.Code)
	}
}

func TestProxyMode(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	if _, err := New(Config{Mode: ModeProxy, ProxyHeader: "X-Forwarded-User"}, clientip.New(nil), slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("proxy mode without trusted proxies accepted")
	}
	a, err := New(Config{Mode: ModeProxy, ProxyHeader: "X-Forwarded-User", ProxyAllowed: []string{"erik"}},
		clientip.New(trusted), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	h := a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	cases := []struct {
		remote, user string
		want         int
	}{
		{"10.1.1.1:1", "erik", 200},
		{"203.0.113.5:1", "erik", 401}, // header from an untrusted peer is ignored
		{"10.1.1.1:1", "mallory", 401}, // not in the allow list
		{"10.1.1.1:1", "", 401},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = c.remote
		if c.user != "" {
			req.Header.Set("X-Forwarded-User", c.user)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("remote %s user %q: %d, want %d", c.remote, c.user, rec.Code, c.want)
		}
	}
	if rec := login(a, "x"); rec.Code != 404 {
		t.Errorf("password login in proxy mode: %d", rec.Code)
	}
}
