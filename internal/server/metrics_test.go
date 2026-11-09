package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetrics(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	base := "/api/somas/" + testSoma
	expect(t, admin.do("POST", base+"/notes/Secret%20plan.md", `{"content":"x"}`), 201, "")
	expect(t, admin.do("GET", base+"/notes/Secret%20plan.md", ""), 200, "")
	newClient(t, s).do("POST", "/api/auth/login", `{"username":"admin","password":"wrong password"}`)

	rec := httptest.NewRecorder()
	s.MetricsHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("metrics: %d %v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`dendronotes_http_requests_total{route="GET /api/somas/{soma}/notes/{path...}",method="GET",code="200"} 1`,
		`dendronotes_http_requests_total{route="POST /api/somas/{soma}/notes/{path...}",method="POST",code="201"} 1`,
		`dendronotes_http_request_duration_seconds_count{route="POST /api/auth/login"}`,
		`dendronotes_logins_total{via="password",result="success"} 1`,
		`dendronotes_logins_total{via="password",result="failure"} 1`,
		`dendronotes_accounts{source="local",state="enabled"} 1`,
		`# TYPE dendronotes_somas gauge`,
		`dendronotes_build_info{version="test"`,
		`# TYPE go_goroutines gauge`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "Secret") {
		t.Error("a note's path became a label")
	}
	// Only /metrics, only GET.
	rec = httptest.NewRecorder()
	s.MetricsHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/somas", nil))
	if rec.Code != 404 {
		t.Errorf("other path on the metrics port: %d", rec.Code)
	}
}
