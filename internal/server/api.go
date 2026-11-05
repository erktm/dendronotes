package server

import (
	"errors"
	"net/http"
	"strings"

	"dendronotes/internal/httpx"
	"dendronotes/internal/soma"
)

// route is one endpoint. The table is the single source of truth for the
// HTTP surface; a test checks it against docs/openapi.yaml.
type route struct {
	method  string
	pattern string
	handler http.HandlerFunc
	public  bool // reachable without authentication
}

// routes lists all endpoints except the frontend assets. Everything that
// works on the soma lives below /api/soma/ and requires a signed-in user.
func (s *Server) routes() []route {
	return []route{
		{"GET", "/healthz", s.handleHealthz, true},
		{"GET", "/readyz", s.handleReadyz, true},

		{"POST", "/api/auth/login", s.auth.HandleLogin, true},
		{"POST", "/api/auth/logout", s.auth.HandleLogout, true},
		{"GET", "/api/auth/me", s.auth.HandleMe, true},

		{"GET", "/api/soma/tree", s.handleTree, false},
		{"GET", "/api/soma/notes/{path...}", s.handleReadNote, false},
		{"PUT", "/api/soma/notes/{path...}", s.handleWriteNote, false},
		{"POST", "/api/soma/notes/{path...}", s.handleCreateNote, false},
		{"POST", "/api/soma/folders/{path...}", s.handleMkdir, false},
		{"POST", "/api/soma/move", s.handleMove, false},
		{"DELETE", "/api/soma/entries/{path...}", s.handleTrash, false},
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	for _, rt := range s.routes() {
		var h http.Handler = rt.handler
		if !rt.public {
			h = s.auth.Require(h)
		}
		mux.Handle(rt.method+" "+rt.pattern, h)
	}
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	entries, err := s.soma.Tree()
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) handleReadNote(w http.ResponseWriter, r *http.Request) {
	n, err := s.soma.ReadNote(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteETag(n.ETag))
	httpx.WriteJSON(w, http.StatusOK, n)
}

// noteMeta is a note without its content (the client already has it).
type noteMeta struct {
	Path  string `json:"path"`
	ETag  string `json:"etag"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

func metaOf(n soma.Note) noteMeta {
	return noteMeta{Path: n.Path, ETag: n.ETag, Size: n.Size, MTime: n.MTime}
}

// handleWriteNote saves a note. If-Match is mandatory, so a client can never
// overwrite a version it has not seen (lost update protection).
func (s *Server) handleWriteNote(w http.ResponseWriter, r *http.Request) {
	etag, ok := parseIfMatch(r.Header.Get("If-Match"))
	if !ok {
		httpx.Error(w, http.StatusPreconditionRequired, "if_match_required")
		return
	}
	var body struct {
		Content *string `json:"content"`
	}
	if err := httpx.DecodeJSON(w, r, &body, s.maxNoteBody()); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if body.Content == nil {
		httpx.Error(w, http.StatusBadRequest, "content_required")
		return
	}
	n, err := s.soma.WriteNote(r.PathValue("path"), []byte(*body.Content), etag)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteETag(n.ETag))
	httpx.WriteJSON(w, http.StatusOK, metaOf(n))
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
	}
	if err := httpx.DecodeJSON(w, r, &body, s.maxNoteBody()); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	n, err := s.soma.CreateNote(r.PathValue("path"), []byte(body.Content))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteETag(n.ETag))
	httpx.WriteJSON(w, http.StatusCreated, metaOf(n))
}

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	e, err := s.soma.Mkdir(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, e)
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	to, err := s.soma.Move(body.From, body.To)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"path": to})
}

func (s *Server) handleTrash(w http.ResponseWriter, r *http.Request) {
	dst, err := s.soma.Trash(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"trashedTo": dst})
}

// maxNoteBody bounds a JSON body carrying a note: JSON escaping can blow
// up text by up to 6× (\uXXXX).
func (s *Server) maxNoteBody() int64 { return s.cfg.MaxNoteBytes*6 + 4096 }

// somaError maps soma errors to HTTP responses with stable error codes.
func (s *Server) somaError(w http.ResponseWriter, r *http.Request, err error) {
	var ce *soma.ConflictError
	switch {
	case errors.As(err, &ce):
		w.Header().Set("ETag", quoteETag(ce.ETag))
		httpx.WriteJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "conflict", "etag": ce.ETag})
	case errors.Is(err, soma.ErrInvalidPath):
		httpx.Error(w, http.StatusBadRequest, "invalid_path")
	case errors.Is(err, soma.ErrInvalidName):
		httpx.Error(w, http.StatusBadRequest, "invalid_name")
	case errors.Is(err, soma.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, soma.ErrExists):
		httpx.Error(w, http.StatusConflict, "exists")
	case errors.Is(err, soma.ErrNotNote):
		httpx.Error(w, http.StatusUnsupportedMediaType, "not_a_note")
	case errors.Is(err, soma.ErrReadOnly):
		httpx.Error(w, http.StatusForbidden, "read_only")
	case errors.Is(err, soma.ErrTooLarge):
		httpx.Error(w, http.StatusRequestEntityTooLarge, "too_large")
	case errors.Is(err, soma.ErrIsDir):
		httpx.Error(w, http.StatusBadRequest, "is_directory")
	case errors.Is(err, soma.ErrNotDir):
		httpx.Error(w, http.StatusBadRequest, "not_a_directory")
	case errors.Is(err, soma.ErrMoveIntoSelf):
		httpx.Error(w, http.StatusBadRequest, "move_into_self")
	default:
		s.log.Error("soma operation failed", "method", r.Method, "path", r.URL.Path, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error")
	}
}

func quoteETag(tag string) string { return `"` + tag + `"` }

// parseIfMatch accepts a single strong ETag ("…"). Weak or wildcard
// validators are not meaningful for content-addressed versions.
func parseIfMatch(h string) (string, bool) {
	h = strings.TrimSpace(h)
	if len(h) < 3 || h[0] != '"' || h[len(h)-1] != '"' {
		return "", false
	}
	tag := h[1 : len(h)-1]
	if strings.ContainsAny(tag, `", `) {
		return "", false
	}
	return tag, true
}
