package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"leafnotes/internal/clientip"
	"leafnotes/internal/config"
)

// client drives the API of a test server like a browser would: it keeps the
// session cookie and sends JSON.
type client struct {
	t      *testing.T
	h      http.Handler
	cookie *http.Cookie
}

func newClient(t *testing.T, s *Server) *client {
	return &client{t: t, h: s.Handler()}
}

func (c *client) do(method, path, body string, headers ...string) *httptest.ResponseRecorder {
	c.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

func (c *client) login() {
	c.t.Helper()
	rec := c.do("POST", "/api/auth/login", `{"password":"`+testPassword+`"}`)
	if rec.Code != 200 {
		c.t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	c.cookie = rec.Result().Cookies()[0]
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body, err)
	}
	return v
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, code int, errCode string) {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("status %d, want %d: %s", rec.Code, code, rec.Body)
	}
	if errCode != "" {
		if got := decode[map[string]string](t, rec)["error"]; got != errCode {
			t.Fatalf("error code %q, want %q", got, errCode)
		}
	}
}

func TestAPIRequiresLogin(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	for _, r := range [][2]string{
		{"GET", "/api/tree"}, {"GET", "/api/files/a.md"}, {"PUT", "/api/files/a.md"},
		{"POST", "/api/files/a.md"}, {"POST", "/api/folders/x"}, {"POST", "/api/move"},
		{"DELETE", "/api/entries/a.md"}, {"GET", "/api/auth/me"},
	} {
		rec := c.do(r[0], r[1], "")
		if rec.Code != 401 {
			t.Errorf("%s %s without session: %d", r[0], r[1], rec.Code)
		}
	}
	if rec := c.do("POST", "/api/auth/login", `{"password":"wrong"}`); rec.Code != 401 {
		t.Fatalf("wrong password: %d", rec.Code)
	}
	c.login()
	me := decode[map[string]string](t, c.do("GET", "/api/auth/me", ""))
	if me["user"] != "owner" || me["mode"] != "password" {
		t.Fatalf("me: %v", me)
	}
	expect(t, c.do("POST", "/api/auth/logout", ""), 200, "")
	expect(t, c.do("GET", "/api/tree", ""), 401, "unauthorized")
}

func TestSessionSurvivesRestart(t *testing.T) {
	cfg := testConfig(t)
	s1 := newTestServerWith(t, testFS(), cfg)
	c := newClient(t, s1)
	c.login()
	s1.close()

	s2 := newTestServerWith(t, testFS(), cfg)
	c.h = s2.Handler()
	expect(t, c.do("GET", "/api/tree", ""), 200, "")
}

func TestCrossOriginWritesAreRejected(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	c.login()
	for _, h := range [][]string{
		{"Sec-Fetch-Site", "cross-site"},
		{"Sec-Fetch-Site", "same-site"},
		{"Origin", "https://evil.example"},
	} {
		expect(t, c.do("POST", "/api/folders/Attack", `{}`, h...), 403, "cross_origin_request")
		expect(t, c.do("POST", "/api/auth/login", `{"password":"x"}`, h...), 403, "cross_origin_request")
	}
	// Same-origin browser requests pass.
	expect(t, c.do("POST", "/api/folders/Fine", `{}`, "Sec-Fetch-Site", "same-origin"), 201, "")
	// Reads are not affected.
	expect(t, c.do("GET", "/api/tree", "", "Sec-Fetch-Site", "cross-site"), 200, "")
}

func TestNoteLifecycle(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	c.login()

	expect(t, c.do("POST", "/api/folders/Projects", ``), 201, "")
	rec := c.do("POST", "/api/files/Projects/Idea.md", `{"content":"# Idea\n"}`)
	expect(t, rec, 201, "")
	etag := decode[map[string]any](t, rec)["etag"].(string)
	expect(t, c.do("POST", "/api/files/Projects/Idea.md", `{"content":""}`), 409, "exists")

	rec = c.do("GET", "/api/files/Projects/Idea.md", "")
	expect(t, rec, 200, "")
	if rec.Header().Get("ETag") != `"`+etag+`"` {
		t.Fatalf("ETag header %q, want %q", rec.Header().Get("ETag"), etag)
	}
	if n := decode[map[string]any](t, rec); n["content"] != "# Idea\n" {
		t.Fatalf("content: %v", n)
	}

	// Saving requires If-Match …
	expect(t, c.do("PUT", "/api/files/Projects/Idea.md", `{"content":"v2"}`), 428, "if_match_required")
	// … with the current version.
	rec = c.do("PUT", "/api/files/Projects/Idea.md", `{"content":"v2"}`, "If-Match", `"`+etag+`"`)
	expect(t, rec, 200, "")
	etag2 := decode[map[string]any](t, rec)["etag"].(string)
	rec = c.do("PUT", "/api/files/Projects/Idea.md", `{"content":"v3"}`, "If-Match", `"`+etag+`"`)
	expect(t, rec, 412, "conflict")
	if decode[map[string]string](t, rec)["etag"] != etag2 {
		t.Fatal("conflict response lacks the current ETag")
	}

	// Move and rename.
	expect(t, c.do("POST", "/api/move", `{"from":"Projects/Idea.md","to":"Projects/Plan.md"}`), 200, "")
	expect(t, c.do("POST", "/api/move", `{"from":"Projects/Plan.md","to":"Missing/Plan.md"}`), 404, "not_found")
	expect(t, c.do("POST", "/api/move", `{"from":"Projects","to":"Projects/Sub/Projects"}`), 400, "move_into_self")

	tree := decode[struct {
		Entries []struct{ Path, Type string }
	}](t, c.do("GET", "/api/tree", ""))
	if len(tree.Entries) != 2 || tree.Entries[1].Path != "Projects/Plan.md" {
		t.Fatalf("tree: %+v", tree.Entries)
	}

	// Delete moves to the trash inside the vault.
	rec = c.do("DELETE", "/api/entries/Projects/Plan.md", "")
	expect(t, rec, 200, "")
	if b, err := os.ReadFile(filepath.Join(s.cfg.VaultDir, ".trash", "Projects", "Plan.md")); err != nil || string(b) != "v2" {
		t.Fatalf("trashed file: %q %v", b, err)
	}
	expect(t, c.do("GET", "/api/files/Projects/Plan.md", ""), 404, "not_found")
}

func TestAPIRejectsBadInput(t *testing.T) {
	s := newTestServer(t, testFS())
	if err := os.WriteFile(filepath.Join(s.cfg.VaultDir, "pic.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newClient(t, s)
	c.login()

	expect(t, c.do("POST", "/api/files/bad:name.md", `{}`), 400, "invalid_name")
	expect(t, c.do("POST", "/api/files/.hidden.md", `{}`), 400, "invalid_path")
	expect(t, c.do("GET", "/api/files/pic.png", ""), 415, "not_a_note")
	expect(t, c.do("POST", "/api/files/x.md", `{"content":"x","extra":1}`), 400, "bad_request")
	expect(t, c.do("POST", "/api/move", `not json`), 400, "bad_request")
	big := `{"content":"` + strings.Repeat("x", (1<<20)+1) + `"}`
	expect(t, c.do("POST", "/api/files/big.md", big), 413, "too_large")

	// A form post (what a CSRF attack could send) is not accepted as JSON.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/move", strings.NewReader("from=a&to=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(c.cookie)
	c.h.ServeHTTP(rec, req)
	expect(t, rec, 415, "unsupported_media_type")
}

func TestAPICannotLeaveTheVault(t *testing.T) {
	s := newTestServer(t, testFS())
	outside := filepath.Join(filepath.Dir(s.cfg.VaultDir), "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("TOP SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside", filepath.Join(s.cfg.VaultDir, "link")); err != nil {
		t.Skip("symlinks not available")
	}
	c := newClient(t, s)
	c.login()

	for _, p := range []string{
		"/api/files/link/secret.md",
		"/api/files/..%2foutside%2fsecret.md",
		"/api/files/%2e%2e/outside/secret.md",
		"/api/files/..%5coutside%5csecret.md",
		"/api/files/%2Fetc%2Fpasswd",
	} {
		rec := c.do("GET", p, "")
		if rec.Code == 200 || strings.Contains(rec.Body.String(), "TOP SECRET") || strings.Contains(rec.Body.String(), "root:") {
			t.Errorf("GET %s escaped: %d %s", p, rec.Code, rec.Body)
		}
	}
	for _, body := range []string{
		`{"from":"link/secret.md","to":"stolen.md"}`,
		`{"from":"../outside/secret.md","to":"stolen.md"}`,
	} {
		if rec := c.do("POST", "/api/move", body); rec.Code < 400 {
			t.Errorf("move %s succeeded", body)
		}
	}
	if rec := c.do("POST", "/api/files/link/new.md", `{"content":"x"}`); rec.Code < 400 {
		t.Error("create through escaping symlink succeeded")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Fatalf("outside directory was modified: %v", entries)
	}
}

func TestProxyAuthMode(t *testing.T) {
	cfg := testConfig(t)
	cfg.Auth = config.Auth{Mode: "proxy", ProxyHeader: "X-Auth-Request-User", ProxyLogoutURL: "/oauth2/sign_out"}
	trusted, _ := clientip.ParseList("192.0.2.0/24") // httptest's default RemoteAddr is 192.0.2.1
	cfg.TrustedProxies = trusted
	s := newTestServerWith(t, testFS(), cfg)
	c := newClient(t, s)

	expect(t, c.do("GET", "/api/tree", ""), 401, "unauthorized")
	rec := c.do("GET", "/api/auth/me", "", "X-Auth-Request-User", "erik")
	expect(t, rec, 200, "")
	if me := decode[map[string]string](t, rec); me["user"] != "erik" || me["logoutUrl"] != "/oauth2/sign_out" {
		t.Fatalf("me: %v", me)
	}
	expect(t, c.do("GET", "/api/tree", "", "X-Auth-Request-User", "erik"), 200, "")
	expect(t, c.do("POST", "/api/auth/login", `{"password":"x"}`), 404, "not_available")
}
