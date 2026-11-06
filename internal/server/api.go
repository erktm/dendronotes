package server

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"

	"dendronotes/internal/httpx"
	"dendronotes/internal/relink"
	"dendronotes/internal/soma"
	"dendronotes/internal/somas"
)

// route is one endpoint. The table is the single source of truth for the
// HTTP surface; a test checks it against docs/openapi.yaml.
type route struct {
	method  string
	pattern string
	handler http.HandlerFunc
	access  access
}

func (s *Server) routes() []route {
	return []route{
		{"GET", "/healthz", s.handleHealthz, public},
		{"GET", "/readyz", s.handleReadyz, public},

		{"POST", "/api/auth/login", s.auth.HandleLogin, public},
		{"POST", "/api/auth/logout", s.auth.HandleLogout, public},
		{"GET", "/api/auth/me", s.auth.HandleMe, public},

		{"PATCH", "/api/me", s.handleUpdateProfile, signedIn},
		{"POST", "/api/me/password", s.handleChangePassword, signedIn},

		{"GET", "/api/somas", s.handleListSomas, signedIn},
		{"POST", "/api/somas", s.handleCreateSoma, signedIn},
		{"PATCH", "/api/somas/{soma}", s.handleRenameSoma, somaOwner},
		{"DELETE", "/api/somas/{soma}", s.handleDeleteSoma, somaOwner},

		{"GET", "/api/somas/{soma}/tree", s.handleTree, somaViewer},
		{"GET", "/api/somas/{soma}/notes/{path...}", s.handleReadNote, somaViewer},
		{"PUT", "/api/somas/{soma}/notes/{path...}", s.handleWriteNote, somaEditor},
		{"POST", "/api/somas/{soma}/notes/{path...}", s.handleCreateNote, somaEditor},
		{"POST", "/api/somas/{soma}/folders/{path...}", s.handleMkdir, somaEditor},
		{"GET", "/api/somas/{soma}/files/{path...}", s.handleReadFile, somaViewer},
		{"POST", "/api/somas/{soma}/files/{path...}", s.handleUpload, somaEditor},
		{"POST", "/api/somas/{soma}/move", s.handleMove, somaEditor},
		{"DELETE", "/api/somas/{soma}/entries/{path...}", s.handleTrash, somaEditor},
		{"GET", "/api/somas/{soma}/search", s.handleSearch, somaViewer},
		{"GET", "/api/somas/{soma}/backlinks/{path...}", s.handleBacklinks, somaViewer},
		{"GET", "/api/somas/{soma}/tags", s.handleTags, somaViewer},
		{"GET", "/api/somas/{soma}/graph", s.handleGraph, somaViewer},
		{"GET", "/api/somas/{soma}/events", s.handleEvents, somaViewer},

		{"GET", "/api/admin/users", s.handleAdminUsers, adminOnly},
		{"POST", "/api/admin/users", s.handleAdminCreateUser, adminOnly},
		{"PATCH", "/api/admin/users/{id}", s.handleAdminUpdateUser, adminOnly},
		{"POST", "/api/admin/users/{id}/password", s.handleAdminSetPassword, adminOnly},
		{"DELETE", "/api/admin/users/{id}", s.handleAdminDeleteUser, adminOnly},
		{"DELETE", "/api/admin/users/{id}/sessions", s.handleAdminEndUserSessions, adminOnly},
		{"GET", "/api/admin/somas", s.handleAdminSomas, adminOnly},
		{"PATCH", "/api/admin/somas/{soma}", s.handleAdminTransferSoma, adminOnly},
		{"DELETE", "/api/admin/somas/{soma}", s.handleAdminDeleteSoma, adminOnly},
		{"GET", "/api/admin/sessions", s.handleAdminSessions, adminOnly},
		{"DELETE", "/api/admin/sessions/{id}", s.handleAdminEndSession, adminOnly},
		{"GET", "/api/admin/server", s.handleAdminServer, adminOnly},
		{"GET", "/api/admin/audit", s.handleAdminAudit, adminOnly},
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	for _, rt := range s.routes() {
		var h http.Handler = rt.handler
		switch rt.access {
		case public:
		case signedIn:
			h = s.auth.Require(h)
		case adminOnly:
			h = s.auth.RequireAdmin(h)
		default:
			h = s.auth.Require(s.withSoma(rt.access.somaRole(), h))
		}
		mux.Handle(rt.method+" "+rt.pattern, h)
	}
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	entries, err := rt.Soma.Tree()
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) handleReadNote(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	n, err := rt.Soma.ReadNote(r.PathValue("path"))
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
	rt := runtimeOf(r)
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
	n, err := rt.Soma.WriteNote(r.PathValue("path"), []byte(*body.Content), etag)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	if ch, err := rt.Index.Put(context.WithoutCancel(r.Context()), n); err != nil {
		s.log.Error("index update after save failed", "path", n.Path, "err", err)
	} else {
		rt.Publish(ch)
	}
	w.Header().Set("ETag", quoteETag(n.ETag))
	httpx.WriteJSON(w, http.StatusOK, metaOf(n))
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	var body struct {
		Content string `json:"content"`
	}
	if err := httpx.DecodeJSON(w, r, &body, s.maxNoteBody()); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	n, err := rt.Soma.CreateNote(r.PathValue("path"), []byte(body.Content))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	rt.SyncAfterWrite(r.Context())
	w.Header().Set("ETag", quoteETag(n.ETag))
	httpx.WriteJSON(w, http.StatusCreated, metaOf(n))
}

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	e, err := rt.Soma.Mkdir(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	rt.SyncAfterWrite(r.Context())
	httpx.WriteJSON(w, http.StatusCreated, e)
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	var body struct {
		From           string `json:"from"`
		To             string `json:"to"`
		UpdateSynapses bool   `json:"updateSynapses"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if !body.UpdateSynapses {
		to, err := rt.Soma.Move(body.From, body.To)
		if err != nil {
			s.somaError(w, r, err)
			return
		}
		rt.SyncAfterWrite(r.Context())
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"path": to})
		return
	}

	rt.MoveMu.Lock()
	defer rt.MoveMu.Unlock()
	edits, err := s.planRelink(r, rt, body.From, body.To)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	to, err := rt.Soma.Move(body.From, body.To)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	res := moveResult{Path: to, Failed: []string{}}
	for _, e := range edits {
		// Based on the version read before the move: a note edited in the
		// meantime is reported, never overwritten.
		if _, err := rt.Soma.WriteNote(e.Path, []byte(e.Content), e.ETag); err != nil {
			s.log.Warn("synapse update skipped", "path", e.Path, "err", err)
			res.Failed = append(res.Failed, e.Path)
			continue
		}
		res.Notes++
		res.Synapses += e.Synapses
	}
	rt.SyncAfterWrite(r.Context())
	httpx.WriteJSON(w, http.StatusOK, res)
}

// moveResult reports a move and the synapses rewritten for it.
type moveResult struct {
	Path     string   `json:"path"`
	Notes    int      `json:"updatedNotes"`
	Synapses int      `json:"updatedSynapses"`
	Failed   []string `json:"failed"`
}

// planRelink computes the synapse rewrites for moving from to to: a single
// note or a folder with all notes inside it.
func (s *Server) planRelink(r *http.Request, rt *somas.Runtime, from, to string) ([]relink.Edit, error) {
	from, err := soma.CleanPath(from)
	if err != nil {
		return nil, err
	}
	to, err = soma.CleanPath(to)
	if err != nil {
		return nil, err
	}
	notes := rt.Index.Resolver().Notes()
	moves := map[string]string{}
	for _, n := range notes {
		switch {
		case n == from:
			moves[n] = to
		case strings.HasPrefix(n, from+"/"):
			moves[n] = to + n[len(from):]
		}
	}
	if len(moves) == 0 {
		return nil, nil // no notes involved (an attachment or an empty folder)
	}
	sources, err := rt.Index.Referrers(r.Context(), slices.Collect(maps.Keys(moves)))
	if err != nil {
		return nil, err
	}
	read := func(p string) (string, string, error) {
		n, err := rt.Soma.ReadNote(p)
		return n.Content, n.ETag, err
	}
	return relink.Plan(read, sources, notes, moves)
}

func (s *Server) handleTrash(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	dst, err := rt.Soma.Trash(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	rt.SyncAfterWrite(r.Context())
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
	case errors.Is(err, soma.ErrType):
		httpx.Error(w, http.StatusUnsupportedMediaType, "unsupported_type")
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
