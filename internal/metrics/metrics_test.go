package metrics

import (
	"strings"
	"testing"
)

func TestTextFormat(t *testing.T) {
	r := New()
	c := r.Counter("app_requests_total", "Requests by route.", "route", "code")
	c.Inc("GET /a", "200")
	c.Inc("GET /a", "200")
	c.Add(3, `GET /b"x\y`, "404")
	h := r.Histogram("app_seconds", "Durations.", []float64{0.5, 0.1}, "route")
	h.Observe(0.05, "GET /a")
	h.Observe(0.3, "GET /a")
	h.Observe(2, "GET /a")
	r.Collect(func(w *Writer) {
		w.Gauge("app_up", "Whether it works.\nReally.", 1)
		w.Family("app_accounts", "gauge", "Accounts by source.")
		w.Sample("app_accounts", 2, "source", "local")
	})
	var b strings.Builder
	if _, err := r.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	want := `# HELP app_requests_total Requests by route.
# TYPE app_requests_total counter
app_requests_total{route="GET /a",code="200"} 2
app_requests_total{route="GET /b\"x\\y",code="404"} 3
# HELP app_seconds Durations.
# TYPE app_seconds histogram
app_seconds_bucket{route="GET /a",le="0.1"} 1
app_seconds_bucket{route="GET /a",le="0.5"} 2
app_seconds_bucket{route="GET /a",le="+Inf"} 3
app_seconds_sum{route="GET /a"} 2.35
app_seconds_count{route="GET /a"} 3
# HELP app_up Whether it works.\nReally.
# TYPE app_up gauge
app_up 1
# HELP app_accounts Accounts by source.
# TYPE app_accounts gauge
app_accounts{source="local"} 2
`
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestWrongLabelCountPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	New().Counter("x", "x", "a").Inc()
}
