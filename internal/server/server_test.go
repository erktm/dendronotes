package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/config"
)

const testIndex = `<!doctype html><html><head><meta name="csp-nonce" content="__CSP_NONCE__"></head><body>app</body></html>`

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":               {Data: []byte(testIndex)},
		"favicon.svg":              {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
		"assets/index-abc123.js":   {Data: []byte("console.log(1)")},
		"assets/index-abc123.css":  {Data: []byte("body{}")},
		"assets/inter-latin.woff2": {Data: []byte("wOF2")},
		".keep":                    {Data: nil},
	}
}

const testPassword = "test password"

func init() {
	// Cheap argon2 parameters keep the tests fast.
	hashParams = auth.Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	base := t.TempDir()
	return config.Config{
		SomaDir:         filepath.Join(base, "soma"),
		DataDir:         filepath.Join(base, "data"),
		Port:            0,
		LogLevel:        slog.LevelInfo,
		LogFormat:       "json",
		ShutdownTimeout: 5 * time.Second,
		MaxNoteBytes:    1 << 20,
		Auth: config.Auth{
			Mode:         "password",
			Password:     testPassword,
			SessionTTL:   time.Hour,
			CookieSecure: "auto",
			ProxyHeader:  "X-Forwarded-User",
		},
	}
}

func newTestServer(t *testing.T, fsys fstest.MapFS) *Server {
	t.Helper()
	return newTestServerWith(t, fsys, testConfig(t))
}

func newTestServerWith(t *testing.T, fsys fstest.MapFS, cfg config.Config) *Server {
	t.Helper()
	for _, d := range []string{cfg.SomaDir, cfg.DataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(cfg, slog.New(slog.DiscardHandler), fsys, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.close() })
	return s
}

func get(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

var nonceInCSP = regexp.MustCompile(`style-src 'self' 'nonce-([A-Za-z0-9_-]+)'`)

func TestIndexHasSecurityHeadersAndMatchingNonce(t *testing.T) {
	s := newTestServer(t, testFS())
	rec := get(t, s.Handler(), "GET", "/")

	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	m := nonceInCSP.FindStringSubmatch(csp)
	if m == nil {
		t.Fatalf("no nonce in CSP: %q", csp)
	}
	if !strings.Contains(rec.Body.String(), `content="`+m[1]+`"`) {
		t.Errorf("nonce in HTML does not match the CSP")
	}
	if strings.Contains(rec.Body.String(), noncePlaceholder) {
		t.Errorf("placeholder not replaced")
	}
	for _, want := range []string{"script-src 'self';", "frame-ancestors 'none'", "object-src 'none'", "base-uri 'none'", "default-src 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP lacks %q", want)
		}
	}
	for _, bad := range []string{"unsafe-inline", "unsafe-eval", "http:", "https:", "*"} {
		if strings.Contains(csp, bad) {
			t.Errorf("CSP contains %q", bad)
		}
	}
	wantHeaders := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Cache-Control":                "no-store",
		"Content-Type":                 "text/html; charset=utf-8",
	}
	for k, v := range wantHeaders {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestNonceUniquePerRequest(t *testing.T) {
	s := newTestServer(t, testFS())
	h := s.Handler()
	a := get(t, h, "GET", "/").Header().Get("Content-Security-Policy")
	b := get(t, h, "GET", "/").Header().Get("Content-Security-Policy")
	if a == b {
		t.Fatal("nonce is reused")
	}
}

func TestStaticAssets(t *testing.T) {
	s := newTestServer(t, testFS())
	h := s.Handler()

	cases := []struct {
		path, wantType, wantCache string
	}{
		{"/assets/index-abc123.js", "text/javascript; charset=utf-8", "immutable"},
		{"/assets/index-abc123.css", "text/css; charset=utf-8", "immutable"},
		{"/assets/inter-latin.woff2", "font/woff2", "immutable"},
		{"/favicon.svg", "image/svg+xml", "max-age=3600"},
	}
	for _, c := range cases {
		rec := get(t, h, "GET", c.path)
		if rec.Code != 200 {
			t.Errorf("%s: status %d", c.path, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != c.wantType {
			t.Errorf("%s: Content-Type %q, want %q", c.path, got, c.wantType)
		}
		if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, c.wantCache) {
			t.Errorf("%s: Cache-Control %q", c.path, got)
		}
	}
}

func TestMissingAssetIs404NotIndex(t *testing.T) {
	s := newTestServer(t, testFS())
	rec := get(t, s.Handler(), "GET", "/assets/missing.js")
	if rec.Code != 404 {
		t.Fatalf("status %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "csp-nonce") {
		t.Fatal("served index.html instead of 404 (MIME confusion)")
	}
}

func TestSPAFallbackAndHiddenFiles(t *testing.T) {
	s := newTestServer(t, testFS())
	h := s.Handler()
	for _, p := range []string{"/note/Projects/Idea.md", "/.keep", "/unknown", "/assets"} {
		rec := get(t, h, "GET", p)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "csp-nonce") {
			t.Errorf("%s: expected index.html, got %d %q", p, rec.Code, rec.Body.String())
		}
	}
}

func TestTraversalOnStaticFiles(t *testing.T) {
	s := newTestServer(t, testFS())
	h := s.Handler()
	for _, p := range []string{"/assets/../../../etc/passwd", "/assets/%2e%2e/%2e%2e/etc/passwd", "/..%2f..%2fetc%2fpasswd"} {
		rec := get(t, h, "GET", p)
		if strings.Contains(rec.Body.String(), "root:") {
			t.Errorf("%s: served a file outside the asset root", p)
		}
	}
}

func TestAPIUnknownIsJSON404(t *testing.T) {
	s := newTestServer(t, testFS())
	rec := get(t, s.Handler(), "GET", "/api/missing")
	if rec.Code != 404 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestMethodNotAllowedOnUI(t *testing.T) {
	s := newTestServer(t, testFS())
	rec := get(t, s.Handler(), "POST", "/")
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("status %d, Allow %q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestFrontendMissing(t *testing.T) {
	s := newTestServer(t, fstest.MapFS{".keep": {}})
	rec := get(t, s.Handler(), "GET", "/")
	if rec.Code != 503 {
		t.Fatalf("status %d, want 503", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t, testFS())
	rec := get(t, s.Handler(), "GET", "/healthz")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
}

func readyStatus(t *testing.T, h http.Handler) (int, map[string]string) {
	t.Helper()
	rec := get(t, h, "GET", "/readyz")
	var body struct {
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("readyz JSON: %v", err)
	}
	return rec.Code, body.Checks
}

func TestReadyzLifecycle(t *testing.T) {
	s := newTestServer(t, testFS())
	h := s.Handler()

	if code, checks := readyStatus(t, h); code != 503 || checks["server"] != "starting" || checks["index"] != "building" {
		t.Fatalf("before start: %d %v", code, checks)
	}
	s.ready.Store(true)
	if code, checks := readyStatus(t, h); code != 503 || checks["index"] != "building" {
		t.Fatalf("before the first index sync: %d %v", code, checks)
	}
	if _, err := s.index.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if code, _ := readyStatus(t, h); code != 200 {
		t.Fatalf("after start: %d", code)
	}

	// The soma disappears (volume unmounted) → no longer ready.
	if err := os.Remove(s.cfg.SomaDir); err != nil {
		t.Fatal(err)
	}
	if code, checks := readyStatus(t, h); code != 503 || checks["soma"] != "unavailable" {
		t.Fatalf("without soma: %d %v", code, checks)
	}
	if err := os.Mkdir(s.cfg.SomaDir, 0o755); err != nil {
		t.Fatal(err)
	}

	s.draining.Store(true)
	if code, checks := readyStatus(t, h); code != 503 || checks["server"] != "shutting_down" {
		t.Fatalf("while shutting down: %d %v", code, checks)
	}
}

func TestPanicIsRecovered(t *testing.T) {
	s := newTestServer(t, testFS())
	h := s.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := get(t, h, "GET", "/")
	if rec.Code != 500 {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("security headers missing on 500")
	}
}

// TestGracefulShutdown checks that a request in flight when the shutdown
// signal arrives is still answered completely and Serve returns cleanly.
func TestGracefulShutdown(t *testing.T) {
	s := newTestServer(t, testFS())
	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		io.WriteString(w, "done")
	})
	mux.Handle("/", s.Handler())
	s.httpSrv.Handler = s.wrap(mux)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()

	base := "http://" + ln.Addr().String()
	waitReady(t, base)

	type result struct {
		body string
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := noKeepAlive.Get(base + "/slow")
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		resCh <- result{string(b), err}
	}()

	<-started
	cancel() // equivalent to SIGTERM

	r := <-resCh
	if r.err != nil || r.body != "done" {
		t.Fatalf("in-flight request aborted: %q %v", r.body, r.err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve does not return")
	}
	if s.ready.Load() || !s.draining.Load() {
		t.Error("wrong readiness state after shutdown")
	}
}

func waitReady(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := noKeepAlive.Get(base + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not become ready")
}

// noKeepAlive avoids pooled spare connections: the server treats a connection
// that never sent a request as busy for up to 5 s during shutdown, which made
// TestGracefulShutdown flaky under load.
var noKeepAlive = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
