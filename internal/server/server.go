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
	"sync/atomic"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/clientip"
	"dendronotes/internal/config"
	"dendronotes/internal/httpx"
	"dendronotes/internal/soma"
	"dendronotes/internal/somas"
	"dendronotes/internal/store"
)

// hashParams are the argon2id parameters for new passwords. The zero value
// means auth.DefaultParams; tests lower them for speed.
var hashParams auth.Params

// Server bundles the HTTP server, its dependencies and the readiness state.
type Server struct {
	cfg     config.Config
	log     *slog.Logger
	store   *store.Store
	somas   *somas.Manager
	auth    *auth.Authenticator
	ips     *clientip.Resolver
	assets  *assets
	version string
	started time.Time

	httpSrv  *http.Server
	ready    atomic.Bool // true only after a complete startup
	draining atomic.Bool // true from SIGTERM on

	// closers run in reverse order during shutdown, once no requests are in
	// flight anymore (somas, then the state database).
	closers []namedCloser
}

type namedCloser struct {
	name string
	fn   func() error
}

// New builds the server. webFS contains the built frontend (index.html, assets/ …).
func New(cfg config.Config, log *slog.Logger, webFS fs.FS, version string) (*Server, error) {
	st, err := store.Open(cfg.DataDir, log)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, log: log, store: st, ips: clientip.New(cfg.TrustedProxies), version: version, started: time.Now()}
	s.addCloser("state", st.Close)
	fail := func(err error) (*Server, error) {
		s.close()
		return nil, err
	}

	s.auth, err = auth.New(auth.Config{
		Mode:           auth.Mode(cfg.Auth.Mode),
		SessionTTL:     cfg.Auth.SessionTTL,
		CookieSecure:   cfg.Auth.CookieSecure,
		HashParams:     hashParams,
		AdminUsername:  cfg.Auth.AdminUsername,
		ProxyHeader:    cfg.Auth.ProxyHeader,
		ProxyAllowed:   cfg.Auth.ProxyAllowed,
		ProxyLogoutURL: cfg.Auth.ProxyLogoutURL,
		OnProvision:    s.welcome,
	}, st, s.ips, log)
	if err != nil {
		return fail(fmt.Errorf("auth: %w", err))
	}

	s.somas = somas.New(st, somas.Options{
		SomasDir:       cfg.SomasDir,
		DataDir:        cfg.DataDir,
		Soma:           soma.Options{Ignore: cfg.Ignore, MaxNoteBytes: cfg.MaxNoteBytes},
		RescanInterval: cfg.RescanInterval,
		Log:            log,
	})
	s.addCloser("somas", s.somas.Close)

	ctx := context.Background()
	if err := s.auth.Bootstrap(ctx, cfg.Auth.AdminHash, cfg.Auth.AdminPassword.Reveal()); err != nil {
		return fail(err)
	}
	if err := s.firstSoma(ctx); err != nil {
		return fail(err)
	}

	if s.assets, err = newAssets(webFS); err != nil {
		return fail(err)
	}
	if !s.assets.hasIndex() {
		log.Warn("frontend not built: web/dist/index.html missing, UI will answer 503")
	}

	s.httpSrv = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	// Event streams never become idle; end them as soon as shutdown begins.
	s.httpSrv.RegisterOnShutdown(s.somas.Shutdown)
	return s, nil
}

// firstSoma gives the first admin a soma when there is none yet: the folder
// of DENDRONOTES_SOMA_DIR (an upgrade from a single-soma installation), or a
// new empty one.
func (s *Server) firstSoma(ctx context.Context) error {
	if n, err := s.store.CountSomas(ctx); err != nil || n > 0 {
		return err
	}
	admin, err := s.store.UserByUsername(ctx, s.cfg.Auth.AdminUsername)
	if errors.Is(err, store.ErrNotFound) {
		return nil // proxy mode: the admin appears with the first request
	}
	if err != nil {
		return err
	}
	if s.cfg.ImportSomaDir != "" {
		sm, err := s.somas.Import(ctx, s.cfg.ImportSomaDir, "Notes", admin.ID)
		if err != nil {
			return fmt.Errorf("import %s: %w", s.cfg.ImportSomaDir, err)
		}
		s.log.Info("existing soma registered for the admin", "dir", s.cfg.ImportSomaDir, "soma", sm.ID)
		return nil
	}
	return s.welcome(ctx, admin)
}

// welcome gives a new account its first, empty soma.
func (s *Server) welcome(ctx context.Context, u store.User) error {
	sm, err := s.somas.Create(ctx, "Notes", u.ID)
	if err != nil {
		s.log.Error("creating the first soma failed", "user", u.Username, "err", err)
		return err
	}
	s.store.Audit(ctx, store.AuditEntry{ActorID: u.ID, Actor: u.Username, Action: "soma.create", Target: sm.ID, Detail: sm.Name})
	return nil
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

	s.ready.Store(true)
	s.log.Info("listening",
		"addr", ln.Addr().String(),
		"auth", s.cfg.Auth.Mode,
		"somas", s.cfg.SomasDir,
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
