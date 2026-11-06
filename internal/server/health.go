package server

import (
	"net/http"

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
func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
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

	// Traffic only once the index has caught up with the soma, so that search
	// and backlinks are complete.
	if s.index.Ready() {
		checks["index"] = "ok"
	} else {
		checks["index"] = "building"
		ok = false
	}

	if err := s.soma.Check(); err != nil {
		s.log.Warn("readiness: soma unavailable", "err", err)
		checks["soma"] = "unavailable"
		ok = false
	} else {
		checks["soma"] = "ok"
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
