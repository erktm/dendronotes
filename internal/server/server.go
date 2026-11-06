// Package server provides HTTP routes, middleware and the server lifecycle.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/clientip"
	"dendronotes/internal/config"
	"dendronotes/internal/events"
	"dendronotes/internal/httpx"
	"dendronotes/internal/index"
	"dendronotes/internal/soma"
)

// hashParams are the argon2id parameters for a plaintext AUTH_PASSWORD_FILE.
// The zero value means auth.DefaultParams; tests lower them for speed.
var hashParams auth.Params

// Server bundles the HTTP server, its dependencies and the readiness state.
type Server struct {
	cfg     config.Config
	log     *slog.Logger
	soma    *soma.Soma
	index   *index.Index
	hub     *events.Hub
	auth    *auth.Authenticator
	assets  *assets
	version string

	httpSrv  *http.Server
	ready    atomic.Bool // true only after a complete startup
	draining atomic.Bool // true from SIGTERM on

	// moveMu serializes moves that rewrite synapses in other notes.
	moveMu sync.Mutex

	// closers run in reverse order during shutdown, once no requests are in
	// flight anymore (background work, index database, soma).
	closers []namedCloser
}

type namedCloser struct {
	name string
	fn   func() error
}

// New builds the server. webFS contains the built frontend (index.html, assets/ …).
func New(cfg config.Config, log *slog.Logger, webFS fs.FS, version string) (*Server, error) {
	authn, err := auth.New(auth.Config{
		Mode:           auth.Mode(cfg.Auth.Mode),
		PasswordHash:   cfg.Auth.PasswordHash,
		Password:       cfg.Auth.Password.Reveal(),
		HashParams:     hashParams,
		SessionsFile:   filepath.Join(cfg.DataDir, "sessions.json"),
		SessionTTL:     cfg.Auth.SessionTTL,
		CookieSecure:   cfg.Auth.CookieSecure,
		ProxyHeader:    cfg.Auth.ProxyHeader,
		ProxyAllowed:   cfg.Auth.ProxyAllowed,
		ProxyLogoutURL: cfg.Auth.ProxyLogoutURL,
	}, clientip.New(cfg.TrustedProxies), log)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	sm, err := soma.Open(cfg.SomaDir, soma.Options{Ignore: cfg.Ignore, MaxNoteBytes: cfg.MaxNoteBytes})
	if err != nil {
		return nil, err
	}
	a, err := newAssets(webFS)
	if err != nil {
		sm.Close()
		return nil, err
	}
	if !a.hasIndex() {
		log.Warn("frontend not built: web/dist/index.html missing, UI will answer 503")
	}

	ix, err := index.Open(cfg.DataDir, sm, log)
	if err != nil {
		sm.Close()
		return nil, fmt.Errorf("index: %w", err)
	}

	s := &Server{cfg: cfg, log: log, soma: sm, index: ix, hub: events.NewHub(maxStreams), auth: authn, assets: a, version: version}
	s.addCloser("soma", sm.Close)
	s.addCloser("index", ix.Close)
	s.httpSrv = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	// Event streams never become idle; end them as soon as shutdown begins.
	s.httpSrv.RegisterOnShutdown(s.hub.Close)
	return s, nil
}

func (s *Server) addCloser(name string, fn func() error) {
	s.closers = append(s.closers, namedCloser{name, fn})
}

// Handler returns the complete HTTP handler including middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	mux.HandleFunc("/api/", s.handleAPINotFound)
	mux.Handle("/", s.assets)

	// CSRF protection for every state-changing request: browsers mark
	// cross-site requests via Sec-Fetch-Site/Origin, and those are refused.
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, http.StatusForbidden, "cross_origin_request")
	}))
	return s.wrap(cop.Handler(mux))
}

// wrap applies the middleware chain to h (outermost first).
func (s *Server) wrap(h http.Handler) http.Handler {
	return s.accessLog(s.recoverPanics(securityHeaders(h)))
}

// Run listens on the configured port until ctx is done.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		s.close()
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve handles requests on ln until ctx is done, then shuts down in order:
// report not ready, let in-flight requests finish, close resources.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.httpSrv.Serve(ln) }()

	bgCtx, stopBackground := context.WithCancel(ctx)
	bgDone := make(chan struct{})
	go func() { defer close(bgDone); s.runIndex(bgCtx) }()
	s.addCloser("background", func() error { stopBackground(); <-bgDone; return nil })

	s.ready.Store(true)
	s.log.Info("listening",
		"addr", ln.Addr().String(),
		"auth", s.cfg.Auth.Mode,
		"soma", s.cfg.SomaDir,
		"data", s.cfg.DataDir,
		"version", s.version,
	)

	select {
	case err := <-errCh:
		s.ready.Store(false)
		s.close()
		return fmt.Errorf("http server stopped: %w", err)
	case <-ctx.Done():
	}

	start := time.Now()
	s.log.Info("shutdown requested, draining", "timeout", s.cfg.ShutdownTimeout.String())
	s.draining.Store(true)
	s.ready.Store(false)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()
	err := s.httpSrv.Shutdown(shutdownCtx)
	if e := <-errCh; e != nil && !errors.Is(e, http.ErrServerClosed) {
		err = errors.Join(err, e)
	}
	err = errors.Join(err, s.close())

	if err != nil {
		s.log.Error("shutdown incomplete", "err", err, "took", time.Since(start).String())
		return err
	}
	s.log.Info("shutdown complete", "took", time.Since(start).String())
	return nil
}

func (s *Server) close() error {
	var errs []error
	for i := len(s.closers) - 1; i >= 0; i-- {
		c := s.closers[i]
		if err := c.fn(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", c.name, err))
		}
	}
	s.closers = nil
	return errors.Join(errs...)
}
