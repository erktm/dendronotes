package server

import (
	"net/http"
	"os"

	"dendronotes/internal/httpx"
)

// handleHealthz is the liveness probe: the process is alive and serving HTTP.
// It stays green during shutdown so Kubernetes does not kill the pod in the
// middle of an orderly drain.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz is the readiness probe: only while it is green does the pod
// receive traffic. Error details go to the log, not into the response
// (the endpoint is unauthenticated).
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{}
	ok := true

	switch {
	case s.draining.Load():
		checks["server"] = "shutting_down"
		ok = false
	case !s.ready.Load():
		checks["server"] = "starting"
		ok = false
	default:
		checks["server"] = "ok"
	}

	if err := s.store.Ping(r.Context()); err != nil {
		s.log.Warn("readiness: state database unavailable", "err", err)
		checks["state"] = "unavailable"
		ok = false
	} else {
		checks["state"] = "ok"
	}
	if st, err := os.Stat(s.cfg.SomasDir); err != nil || !st.IsDir() {
		s.log.Warn("readiness: somas directory unavailable", "err", err)
		checks["somas"] = "unavailable"
		ok = false
	} else {
		checks["somas"] = "ok"
	}

	status, code := "ok", http.StatusOK
	if !ok {
		status, code = "unavailable", http.StatusServiceUnavailable
	}
	httpx.WriteJSON(w, code, map[string]any{"status": status, "checks": checks})
}

func (s *Server) handleAPINotFound(w http.ResponseWriter, _ *http.Request) {
	httpx.Error(w, http.StatusNotFound, "not_found")
}
