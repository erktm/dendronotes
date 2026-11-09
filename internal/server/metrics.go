package server

import (
	"context"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dendronotes/internal/metrics"
)

// Metrics for Prometheus, served on a port of their own
// (DENDRONOTES_METRICS_PORT) that is not exposed like the app. Routes are
// counted by their pattern ("GET /api/somas/{soma}/tree"), so paths of
// notes never become labels.

type serverMetrics struct {
	reg      *metrics.Registry
	requests *metrics.Counter
	duration *metrics.Histogram
	logins   *metrics.Counter

	// Disk usage is expensive: measured in the background, at most every
	// storageEvery.
	storageMu   sync.Mutex
	storageAt   time.Time
	storageBusy bool
	somasBytes  int64
	dataBytes   int64
}

const storageEvery = 10 * time.Minute

type routeKey struct{}

// routeLabel is filled in by the route a request reaches.
type routeLabel struct{ name string }

// labelRoute names the route for the request metrics.
func labelRoute(name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l, ok := r.Context().Value(routeKey{}).(*routeLabel); ok {
			l.name = name
		}
		next.ServeHTTP(w, r)
	})
}

// withRouteLabel prepares a request for labelRoute.
func withRouteLabel(r *http.Request) (*http.Request, *routeLabel) {
	l := &routeLabel{}
	return r.WithContext(context.WithValue(r.Context(), routeKey{}, l)), l
}

var knownMethods = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "OPTIONS": true}

// observe counts a finished request.
func (m *serverMetrics) observe(r *http.Request, l *routeLabel, status int, d time.Duration) {
	route := l.name
	if route == "" {
		route = "static"
		if strings.HasPrefix(r.URL.Path, "/api/") {
			route = "unknown"
		}
	}
	method := r.Method
	if !knownMethods[method] {
		method = "other"
	}
	m.requests.Inc(route, method, strconv.Itoa(status))
	m.duration.Observe(d.Seconds(), route)
}

func (s *Server) newMetrics() *serverMetrics {
	reg := metrics.New()
	m := &serverMetrics{
		reg:      reg,
		requests: reg.Counter("dendronotes_http_requests_total", "HTTP requests by route pattern, method and status.", "route", "method", "code"),
		duration: reg.Histogram("dendronotes_http_request_duration_seconds", "Time to answer HTTP requests by route pattern (event streams last long).",
			[]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}, "route"),
		logins: reg.Counter("dendronotes_logins_total", "Sign-ins by way (password, directory, sso) and result.", "via", "result"),
	}
	reg.Collect(s.collectMetrics)
	return m
}

// login counts a sign-in (auth.Config.OnLogin).
func (m *serverMetrics) login(via string, ok bool) {
	result := "failure"
	if ok {
		result = "success"
	}
	m.logins.Inc(via, result)
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// collectMetrics writes what is gathered at scrape time.
func (s *Server) collectMetrics(w *metrics.Writer) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Family("dendronotes_build_info", "gauge", "Version of dendronotes and Go.")
	w.Sample("dendronotes_build_info", 1, "version", s.version, "go_version", runtime.Version())
	w.Gauge("process_start_time_seconds", "Start time of the process (Unix seconds).", float64(s.started.Unix()))
	w.Gauge("go_goroutines", "Goroutines that currently exist.", float64(runtime.NumGoroutine()))
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	w.Gauge("go_memstats_heap_alloc_bytes", "Bytes of allocated heap objects.", float64(mem.HeapAlloc))

	if users, err := s.store.Users(ctx); err == nil {
		type k struct{ source, state string }
		counts := map[k]int{}
		for _, u := range users {
			state := "enabled"
			if u.Disabled {
				state = "disabled"
			}
			counts[k{string(u.Source), state}]++
		}
		w.Family("dendronotes_accounts", "gauge", "Accounts by source and state.")
		for _, source := range []string{"local", "oidc", "ldap", "proxy"} {
			for _, state := range []string{"enabled", "disabled"} {
				if n := counts[k{source, state}]; n > 0 {
					w.Sample("dendronotes_accounts", float64(n), "source", source, "state", state)
				}
			}
		}
	}
	if sessions, err := s.store.Sessions(ctx); err == nil {
		w.Gauge("dendronotes_sessions", "Valid sign-in sessions.", float64(len(sessions)))
	}
	if n, err := s.store.CountSomas(ctx); err == nil {
		w.Gauge("dendronotes_somas", "Somas.", float64(n))
	}
	running := s.somas.Running()
	streams, docs := 0, 0
	for _, rt := range running {
		streams += rt.Streams()
		docs += len(rt.Collab.Open())
	}
	w.Gauge("dendronotes_somas_open", "Somas open right now (in use recently).", float64(len(running)))
	w.Gauge("dendronotes_live_streams", "Connected live event streams (browser tabs).", float64(streams))
	w.Gauge("dendronotes_notes_open", "Notes open for editing together.", float64(docs))

	somasBytes, dataBytes, at := s.storageUsage()
	if !at.IsZero() {
		w.Family("dendronotes_storage_bytes", "gauge", "Disk usage of the somas and the data folder (measured at most every 10 minutes).")
		w.Sample("dendronotes_storage_bytes", float64(somasBytes), "dir", "somas")
		w.Sample("dendronotes_storage_bytes", float64(dataBytes), "dir", "data")
	}
	if st := s.ssoStatus(); st != nil {
		w.Gauge("dendronotes_oidc_up", "Whether the OpenID Connect provider answered its last check.", boolValue(st.Ready))
	}
	if st := s.ldapState(); st != nil {
		w.Gauge("dendronotes_ldap_up", "Whether the directory answered its last check.", boolValue(st.Ready))
		if sync := s.ldapSyncState(); sync != nil && !sync.LastSuccess.IsZero() {
			w.Gauge("dendronotes_ldap_sync_last_success_timestamp_seconds", "When the directory sync last succeeded (Unix seconds).", float64(sync.LastSuccess.Unix()))
		}
	}
}

// storageUsage returns the last measured disk usage and starts a new
// measurement in the background when it is old.
func (s *Server) storageUsage() (somasBytes, dataBytes int64, at time.Time) {
	m := s.metrics
	m.storageMu.Lock()
	defer m.storageMu.Unlock()
	if !m.storageBusy && time.Since(m.storageAt) > storageEvery {
		m.storageBusy = true
		go func() {
			sb, _ := dirUsage(s.cfg.SomasDir, 2_000_000)
			db, _ := dirUsage(s.cfg.DataDir, 2_000_000)
			m.storageMu.Lock()
			m.somasBytes, m.dataBytes, m.storageAt, m.storageBusy = sb, db, time.Now(), false
			m.storageMu.Unlock()
		}()
	}
	return m.somasBytes, m.dataBytes, m.storageAt
}

// MetricsHandler serves /metrics (GET only).
func (s *Server) MetricsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", metrics.ContentType)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = s.metrics.reg.WriteTo(w)
	})
	return mux
}

// serveMetrics runs the metrics listener until ctx ends (nil without a port).
func (s *Server) serveMetrics(ctx context.Context) error {
	if s.cfg.MetricsPort == 0 {
		return nil
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(s.cfg.MetricsPort))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.MetricsHandler(), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.log.Error("metrics listener stopped", "err", err)
		}
	}()
	s.log.Info("metrics listening", "addr", ln.Addr().String())
	return nil
}
