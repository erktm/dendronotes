package server

import (
	"bytes"
	"errors"
	"net/http"

	"dendronotes/internal/drawing"
	"dendronotes/internal/httpx"
	"dendronotes/internal/soma"
)

// Drawings are files "Name.drawing.svg" in the soma (internal/drawing). The
// editor in the browser reads them as text and saves them whole, guarded by
// a content ETag like notes; the server accepts only what drawing.Check lets
// through.

// drawingPath reads {path} and makes sure it names a drawing.
func drawingPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, err := soma.CleanPath(r.PathValue("path"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_path")
		return "", false
	}
	if !drawing.IsDrawing(p) {
		httpx.Error(w, http.StatusBadRequest, "not_a_drawing")
		return "", false
	}
	return p, true
}

type drawingFile struct {
	Path string `json:"path"`
	SVG  string `json:"svg,omitempty"`
	ETag string `json:"etag"`
}

func (s *Server) handleReadDrawing(w http.ResponseWriter, r *http.Request) {
	p, ok := drawingPath(w, r)
	if !ok {
		return
	}
	data, e, etag, err := runtimeOf(r).Soma.ReadFileContent(p, drawing.MaxBytes)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteETag(etag))
	httpx.WriteJSON(w, http.StatusOK, drawingFile{Path: e.Path, SVG: string(data), ETag: etag})
}

// handleCreateDrawing makes a new, empty drawing; with ?unique=1 a taken
// name gets a " (n)" suffix.
func (s *Server) handleCreateDrawing(w http.ResponseWriter, r *http.Request) {
	p, ok := drawingPath(w, r)
	if !ok {
		return
	}
	rt := runtimeOf(r)
	empty := drawing.Empty()
	e, err := rt.Soma.CreateFile(p, bytes.NewReader(empty), drawing.MaxBytes, r.URL.Query().Get("unique") == "1",
		func([]byte) error { return nil })
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	rt.SyncAfterWrite(r.Context())
	etag := soma.ETag(empty)
	w.Header().Set("ETag", quoteETag(etag))
	httpx.WriteJSON(w, http.StatusCreated, drawingFile{Path: e.Path, ETag: etag})
}

// handleSaveDrawing replaces a drawing: {"svg": …} with If-Match.
func (s *Server) handleSaveDrawing(w http.ResponseWriter, r *http.Request) {
	p, ok := drawingPath(w, r)
	if !ok {
		return
	}
	etag, ok := parseIfMatch(r.Header.Get("If-Match"))
	if !ok {
		httpx.Error(w, http.StatusPreconditionRequired, "if_match_required")
		return
	}
	var body struct {
		SVG string `json:"svg"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 2*drawing.MaxBytes+4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	data := []byte(body.SVG)
	if err := drawing.Check(data); err != nil {
		if errors.Is(err, drawing.ErrInvalid) {
			s.log.Warn("drawing refused", "path", p, "err", err)
			httpx.Error(w, http.StatusBadRequest, "invalid_drawing")
			return
		}
		s.internalError(w, r, "checking drawing", err)
		return
	}
	rt := runtimeOf(r)
	e, newTag, err := rt.Soma.WriteFile(p, data, etag)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	rt.SyncAfterWrite(r.Context())
	w.Header().Set("ETag", quoteETag(newTag))
	httpx.WriteJSON(w, http.StatusOK, drawingFile{Path: e.Path, ETag: newTag})
}
