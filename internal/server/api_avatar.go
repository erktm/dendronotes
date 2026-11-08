package server

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"dendronotes/internal/auth"
	"dendronotes/internal/avatar"
	"dendronotes/internal/httpx"
	"dendronotes/internal/store"
)

// handleSetAvatar stores the designed avatar (color, pattern, initials) of
// the signed-in account.
func (s *Server) handleSetAvatar(w http.ResponseWriter, r *http.Request) {
	var body store.AvatarStyle
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	u, _ := auth.UserFrom(r.Context())
	nu, err := s.store.SetAvatarStyle(r.Context(), u.ID, body)
	if errors.Is(err, store.ErrInvalidAvatar) {
		httpx.Error(w, http.StatusBadRequest, "invalid_avatar")
		return
	}
	if err != nil {
		s.internalError(w, r, "setting avatar", err)
		return
	}
	s.audit(r, "profile.avatar", u.Username, "")
	httpx.WriteJSON(w, http.StatusOK, nu)
}

// handleSetAvatarPhoto stores a profile photo: a square PNG the browser has
// cropped, encoded anew by the server (avatar.Normalize).
func (s *Server) handleSetAvatarPhoto(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "image/png" {
		httpx.Error(w, http.StatusUnsupportedMediaType, "unsupported_type")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, avatar.MaxBytes+1))
	if err != nil || len(data) > avatar.MaxBytes {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	photo, err := avatar.Normalize(data)
	if errors.Is(err, avatar.ErrInvalid) {
		httpx.Error(w, http.StatusBadRequest, "invalid_photo")
		return
	}
	if err != nil {
		s.internalError(w, r, "encoding avatar photo", err)
		return
	}
	u, _ := auth.UserFrom(r.Context())
	nu, err := s.store.SetAvatarPhoto(r.Context(), u.ID, photo)
	if err != nil {
		s.internalError(w, r, "storing avatar photo", err)
		return
	}
	s.audit(r, "profile.photo", u.Username, "")
	httpx.WriteJSON(w, http.StatusOK, nu)
}

// handleRemoveAvatarPhoto deletes the profile photo; the designed avatar
// shows again.
func (s *Server) handleRemoveAvatarPhoto(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	nu, err := s.store.RemoveAvatarPhoto(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, "removing avatar photo", err)
		return
	}
	s.audit(r, "profile.photo.remove", u.Username, "")
	httpx.WriteJSON(w, http.StatusOK, nu)
}

// handleAvatarPhoto delivers the photo of an account – to the account
// itself, to admins and to people it shares a soma with; everyone else gets
// 404 (as if there were none). With ?v= matching the current version the
// answer may be cached for good: a new upload gets a new version.
func (s *Server) handleAvatarPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if u.ID != id && u.Role != store.RoleAdmin {
		shares, err := s.store.SharesSoma(r.Context(), u.ID, id)
		if err != nil {
			s.internalError(w, r, "checking avatar access", err)
			return
		}
		if !shares {
			httpx.Error(w, http.StatusNotFound, "not_found")
			return
		}
	}
	image, version, err := s.store.AvatarPhoto(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "reading avatar photo", err)
		return
	}
	v := strconv.FormatInt(version, 10)
	h := w.Header()
	h.Set("ETag", `"`+v+`"`)
	if r.URL.Query().Get("v") == v {
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "private, no-cache")
	}
	if r.Header.Get("If-None-Match") == `"`+v+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", "image/png")
	h.Set("Content-Security-Policy", fileCSP)
	h.Set("Content-Length", strconv.Itoa(len(image)))
	_, _ = w.Write(image)
}
