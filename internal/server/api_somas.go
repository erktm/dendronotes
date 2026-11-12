package server

import (
	"errors"
	"net/http"
	"strings"

	"dendronotes/internal/auth"
	"dendronotes/internal/httpx"
	"dendronotes/internal/somas"
	"dendronotes/internal/store"
)

// maxSomasPerUser bounds how many somas one account may own.
const maxSomasPerUser = 100

// audit records an action of the signed-in account.
func (s *Server) audit(r *http.Request, action, target, detail string) {
	u, _ := auth.UserFrom(r.Context())
	s.store.Audit(r.Context(), store.AuditEntry{
		ActorID: u.ID, Actor: u.Username, Action: action, Target: target, Detail: detail,
		IP: s.ips.ClientIP(r).String(),
	})
}

// handleListSomas lists the somas of the signed-in account.
func (s *Server) handleListSomas(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	list, err := s.store.SomasOf(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, "listing somas", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"somas": list})
}

// handleCreateSoma creates an empty soma owned by the caller.
func (s *Server) handleCreateSoma(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	name, err := somas.CleanName(body.Name)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_name")
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if owned, err := s.store.SomasOwnedBy(r.Context(), u.ID); err != nil {
		s.internalError(w, r, "counting somas", err)
		return
	} else if len(owned) >= maxSomasPerUser {
		httpx.Error(w, http.StatusConflict, "too_many_somas")
		return
	}
	sm, err := s.somas.Create(r.Context(), name, u.ID)
	if err != nil {
		s.internalError(w, r, "creating soma", err)
		return
	}
	s.audit(r, "soma.create", sm.ID, sm.Name)
	httpx.WriteJSON(w, http.StatusCreated, store.Membership{Soma: sm, Role: store.MemberOwner,
		Owner: store.Person{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName}})
}

// handleUpdateSoma renames a soma or hands it to another member (owner only).
// The previous owner stays as editor.
func (s *Server) handleUpdateSoma(w http.ResponseWriter, r *http.Request) {
	sm, _ := somaOf(r)
	var body struct {
		Name    *string `json:"name"`
		OwnerID *int64  `json:"ownerId"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	name := sm.Name
	if body.Name != nil {
		n, err := somas.CleanName(*body.Name)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid_name")
			return
		}
		name = n
	}
	var heir store.User
	if body.OwnerID != nil && *body.OwnerID != sm.OwnerID {
		role, err := s.store.MemberRole(r.Context(), sm.ID, *body.OwnerID)
		if err != nil {
			s.internalError(w, r, "membership lookup", err)
			return
		}
		if heir, err = s.store.UserByID(r.Context(), *body.OwnerID); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.internalError(w, r, "user lookup", err)
			return
		}
		if role == "" || heir.ID == 0 || heir.Disabled {
			httpx.Error(w, http.StatusBadRequest, "invalid_transfer")
			return
		}
	}
	if name != sm.Name {
		if err := s.store.RenameSoma(r.Context(), sm.ID, name); err != nil {
			s.internalError(w, r, "renaming soma", err)
			return
		}
		s.audit(r, "soma.rename", sm.ID, sm.Name+" → "+name)
	}
	if heir.ID != 0 {
		if err := s.store.TransferSoma(r.Context(), sm.ID, heir.ID); err != nil {
			s.internalError(w, r, "transferring soma", err)
			return
		}
		s.audit(r, "soma.transfer", sm.ID, "→ "+heir.Username)
	}
	accessChanged(r)
	u, _ := auth.UserFrom(r.Context())
	list, err := s.store.SomasOf(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, "listing somas", err)
		return
	}
	for _, m := range list {
		if m.ID == sm.ID {
			httpx.WriteJSON(w, http.StatusOK, m)
			return
		}
	}
	httpx.Error(w, http.StatusNotFound, "not_found")
}

// handleDeleteSoma moves a soma to the trash (owner only).
func (s *Server) handleDeleteSoma(w http.ResponseWriter, r *http.Request) {
	sm, _ := somaOf(r)
	// The request holds the soma open; Delete waits for nothing but takes it
	// out of the manager, so later requests no longer find it.
	me, _ := auth.UserFrom(r.Context())
	if err := s.somas.Delete(r.Context(), sm, me.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.internalError(w, r, "deleting soma", err)
		return
	}
	s.audit(r, "soma.delete", sm.ID, sm.Name)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": sm.ID})
}

// handleUpdateProfile changes the caller's display name and e-mail.
func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName  *string `json:"displayName"`
		Email        *string `json:"email"`
		MailComments *bool   `json:"mailComments"`
		MailShares   *bool   `json:"mailShares"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	up, ok := profileUpdate(w, body.DisplayName, body.Email)
	if !ok {
		return
	}
	up.MailComments, up.MailShares = body.MailComments, body.MailShares
	u, _ := auth.UserFrom(r.Context())
	managed := (u.Source == store.SourceOIDC && s.sso != nil) || u.Source == store.SourceLDAP
	if managed && (body.DisplayName != nil || body.Email != nil) {
		// The provider is the source of name and e-mail; they are updated
		// on every sign-in.
		httpx.Error(w, http.StatusBadRequest, "managed_by_provider")
		return
	}
	nu, err := s.store.UpdateUser(r.Context(), u.ID, up)
	if err != nil {
		s.internalError(w, r, "updating profile", err)
		return
	}
	s.audit(r, "profile.update", u.Username, "")
	s.mailEmailChanged(r.Context(), u, nu, &u)
	httpx.WriteJSON(w, http.StatusOK, nu)
}

// profileUpdate validates display name and e-mail.
func profileUpdate(w http.ResponseWriter, displayName, email *string) (store.UserUpdate, bool) {
	var up store.UserUpdate
	if displayName != nil {
		v := strings.TrimSpace(*displayName)
		if len(v) > 100 || strings.ContainsAny(v, "\r\n\t") {
			httpx.Error(w, http.StatusBadRequest, "invalid_display_name")
			return up, false
		}
		up.DisplayName = &v
	}
	if email != nil {
		v := strings.TrimSpace(*email)
		if len(v) > 254 || (v != "" && (!strings.Contains(v, "@") || strings.ContainsAny(v, " \r\n\t<>"))) {
			httpx.Error(w, http.StatusBadRequest, "invalid_email")
			return up, false
		}
		up.Email = &v
	}
	return up, true
}

// handleChangePassword changes the caller's password (local accounts) and
// ends their other sessions.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if u.Source != store.SourceLocal {
		httpx.Error(w, http.StatusBadRequest, "not_local")
		return
	}
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if !s.auth.CheckPassword(u, body.Current) {
		s.audit(r, "password.change_failed", u.Username, "")
		httpx.Error(w, http.StatusForbidden, "wrong_password")
		return
	}
	if err := auth.ValidPassword(body.New); err != nil {
		httpx.Error(w, http.StatusBadRequest, "weak_password")
		return
	}
	hash, err := s.auth.HashNew(body.New)
	if err != nil {
		s.internalError(w, r, "hashing password", err)
		return
	}
	if _, err := s.store.UpdateUser(r.Context(), u.ID, store.UserUpdate{PasswordHash: &hash}); err != nil {
		s.internalError(w, r, "changing password", err)
		return
	}
	_ = s.store.DeleteUserSessions(r.Context(), u.ID, auth.SessionIDFrom(r.Context()))
	_ = s.store.DropPasswordTokens(r.Context(), u.ID)
	s.audit(r, "password.change", u.Username, "")
	s.mailPasswordChanged(r.Context(), u, "self", nil)
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
