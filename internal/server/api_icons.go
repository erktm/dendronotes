package server

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"dendronotes/internal/auth"
	"dendronotes/internal/drawing"
	"dendronotes/internal/httpx"
	"dendronotes/internal/store"
)

// The icon library of a soma: pictures everyone with access can put into
// drawings. Uploads are PNG (the browser converts other pictures and scales
// them down) or checked SVG; drawings embed their own copy, so deleting an
// icon here changes no drawing.

func (s *Server) handleIcons(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.Icons(r.Context(), runtimeOf(r).ID)
	if err != nil {
		s.internalError(w, r, "listing icons", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"icons": list})
}

// iconID reads {id}.
func iconID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return 0, false
	}
	return id, true
}

// handleIcon serves an icon's picture (sandboxed like every file).
func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	id, ok := iconID(w, r)
	if !ok {
		return
	}
	icon, data, err := s.store.IconByID(r.Context(), runtimeOf(r).ID, id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "reading icon", err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", icon.Mime)
	h.Set("Content-Security-Policy", fileCSP)
	h.Set("Cache-Control", "private, max-age=86400") // an icon never changes
	_, _ = w.Write(data)
}

// handleAddIcon stores the request body as an icon named ?name=.
func (s *Server) handleAddIcon(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "Icon"
	}
	if utf8.RuneCountInString(name) > 80 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_name")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, drawing.MaxIconBytes+1))
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	mime, data, err := drawing.NormalizeIcon(data)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_icon")
		return
	}
	me, _ := auth.UserFrom(r.Context())
	icon, err := s.store.AddIcon(r.Context(), runtimeOf(r).ID, name, mime, data, me.ID)
	if errors.Is(err, store.ErrTooManyIcons) {
		httpx.Error(w, http.StatusConflict, "too_many_icons")
		return
	}
	if err != nil {
		s.internalError(w, r, "adding icon", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, icon)
}

// handleDeleteIcon removes an icon: its uploader, or an editor.
func (s *Server) handleDeleteIcon(w http.ResponseWriter, r *http.Request) {
	id, ok := iconID(w, r)
	if !ok {
		return
	}
	rt := runtimeOf(r)
	icon, _, err := s.store.IconByID(r.Context(), rt.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "reading icon", err)
		return
	}
	me, _ := auth.UserFrom(r.Context())
	if _, role := somaOf(r); icon.CreatedBy != me.ID && store.RoleRank(role) < store.RoleRank(store.MemberEditor) {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.store.DeleteIcon(r.Context(), rt.ID, id); err != nil {
		s.internalError(w, r, "deleting icon", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"id": id})
}
