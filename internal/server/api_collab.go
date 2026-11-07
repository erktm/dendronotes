package server

import (
	"errors"
	"net/http"
	"strconv"

	"dendronotes/internal/auth"
	"dendronotes/internal/collab"
	"dendronotes/internal/httpx"
)

// Editing together. A browser tab joins a note with the id of its event
// stream; from then on it pushes its changes (CodeMirror change sets) for
// the version it has seen and receives everyone's changes, cursors and save
// confirmations on the stream. See internal/collab.

type collabTarget struct {
	Stream string `json:"stream"`
	Path   string `json:"path"`
}

// handleCollabJoin opens a note for editing together.
func (s *Server) handleCollabJoin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		collabTarget
		Recover *collab.Recover `json:"recover"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 2*s.maxNoteBody()+4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	u, _ := auth.UserFrom(r.Context())
	res, err := runtimeOf(r).Collab.Join(body.Stream, u.ID, body.Path, body.Recover)
	if err != nil {
		s.collabError(w, r, err)
		return
	}
	// Viewers (and read-only files) only follow along.
	if _, role := somaOf(r); role == "viewer" {
		res.ReadOnly = true
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// handleCollabLeave closes a note for a tab.
func (s *Server) handleCollabLeave(w http.ResponseWriter, r *http.Request) {
	var body collabTarget
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if err := runtimeOf(r).Collab.Leave(body.Stream, u.ID, body.Path); err != nil {
		s.collabError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleCollabPush applies changes made on top of a version.
func (s *Server) handleCollabPush(w http.ResponseWriter, r *http.Request) {
	var body struct {
		collabTarget
		Doc     string          `json:"doc"`
		Version int             `json:"version"`
		Updates []collab.Update `json:"updates"`
		Save    bool            `json:"save"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 2*s.maxNoteBody()); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if len(body.Updates) > 1000 {
		httpx.Error(w, http.StatusBadRequest, "invalid_update")
		return
	}
	u, _ := auth.UserFrom(r.Context())
	v, err := runtimeOf(r).Collab.Push(body.Stream, u.ID, body.Path, body.Doc, body.Version, body.Updates, body.Save)
	if errors.Is(err, collab.ErrVersion) {
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{"error": "version_mismatch", "version": v})
		return
	}
	if err != nil {
		s.collabError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"version": v})
}

// handleCollabPull returns the updates after a version (catching up).
func (s *Server) handleCollabPull(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	version, err := strconv.Atoi(q.Get("version"))
	if err != nil || version < 0 {
		httpx.Error(w, http.StatusBadRequest, "bad_request")
		return
	}
	updates, v, err := runtimeOf(r).Collab.Pull(q.Get("path"), q.Get("doc"), version)
	if err != nil {
		s.collabError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"updates": updates, "version": v})
}

// handleCollabPresence shares a tab's cursor with the others in the note.
func (s *Server) handleCollabPresence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		collabTarget
		Doc string            `json:"doc"`
		Sel *collab.Selection `json:"sel"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if err := runtimeOf(r).Collab.Presence(body.Stream, u.ID, body.Path, body.Doc, body.Sel); err != nil {
		s.collabError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) collabError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, collab.ErrUnknownStream):
		httpx.Error(w, http.StatusConflict, "unknown_stream")
	case errors.Is(err, collab.ErrResync):
		httpx.Error(w, http.StatusGone, "resync")
	case errors.Is(err, collab.ErrInvalid):
		httpx.Error(w, http.StatusBadRequest, "invalid_update")
	case errors.Is(err, collab.ErrRateLimited):
		httpx.Error(w, http.StatusTooManyRequests, "rate_limited")
	case errors.Is(err, collab.ErrClosed):
		httpx.Error(w, http.StatusServiceUnavailable, "shutting_down")
	default:
		s.somaError(w, r, err)
	}
}
