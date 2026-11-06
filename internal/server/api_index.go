package server

import (
	"net/http"
	"strconv"

	"dendronotes/internal/httpx"
	"dendronotes/internal/soma"
)

const (
	defaultSearchLimit = 50
	maxSearchLimit     = 200
	maxQueryBytes      = 512
)

// handleSearch runs a full-text search. Words match as prefixes, "quoted
// text" as a phrase, #tag filters by tag (nested tags included).
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if len(q) > maxQueryBytes {
		httpx.Error(w, http.StatusBadRequest, "query_too_long")
		return
	}
	limit := defaultSearchLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxSearchLimit {
			httpx.Error(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = n
	}
	hits, err := s.index.Search(r.Context(), q, limit)
	if err != nil {
		s.internalError(w, r, "search", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"hits": hits, "complete": s.index.Ready()})
}

// handleBacklinks lists the notes with synapses to a note ("Incoming synapses").
func (s *Server) handleBacklinks(w http.ResponseWriter, r *http.Request) {
	p, err := soma.CleanPath(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	bl, err := s.index.Backlinks(r.Context(), p)
	if err != nil {
		s.internalError(w, r, "backlinks", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"backlinks": bl})
}

// handleTags lists all tags with the number of notes using them.
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.index.Tags(r.Context())
	if err != nil {
		s.internalError(w, r, "tags", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, what string, err error) {
	if r.Context().Err() == nil {
		s.log.Error(what+" failed", "err", err)
	}
	httpx.Error(w, http.StatusInternalServerError, "internal_error")
}
