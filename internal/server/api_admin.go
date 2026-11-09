package server

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/config"
	"dendronotes/internal/httpx"
	"dendronotes/internal/somas"
	"dendronotes/internal/sso"
	"dendronotes/internal/store"
)

func (s *Server) pathUser(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return store.User{}, false
	}
	u, err := s.store.UserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return store.User{}, false
	}
	if err != nil {
		s.internalError(w, r, "user lookup", err)
		return store.User{}, false
	}
	return u, true
}

// lastAdmin reports whether u is the only enabled admin.
func (s *Server) lastAdmin(ctx context.Context, u store.User) (bool, error) {
	if u.Role != store.RoleAdmin || u.Disabled {
		return false, nil
	}
	n, err := s.store.CountAdmins(ctx)
	return n <= 1, err
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.Users(r.Context())
	if err != nil {
		s.internalError(w, r, "listing users", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": users})
}

// handleAdminCreateUser creates a local account with a first soma.
func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username    string     `json:"username"`
		DisplayName *string    `json:"displayName"`
		Email       *string    `json:"email"`
		Password    string     `json:"password"`
		Role        store.Role `json:"role"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 8192); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	name := strings.TrimSpace(body.Username)
	if auth.ValidUsername(name) != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_username")
		return
	}
	if body.Role != store.RoleAdmin && body.Role != store.RoleUser {
		httpx.Error(w, http.StatusBadRequest, "invalid_role")
		return
	}
	if err := auth.ValidPassword(body.Password); err != nil {
		httpx.Error(w, http.StatusBadRequest, "weak_password")
		return
	}
	up, ok := profileUpdate(w, body.DisplayName, body.Email)
	if !ok {
		return
	}
	hash, err := s.auth.HashNew(body.Password)
	if err != nil {
		s.internalError(w, r, "hashing password", err)
		return
	}
	nu := store.User{Username: name, Role: body.Role, Source: store.SourceLocal, PasswordHash: hash}
	if up.DisplayName != nil {
		nu.DisplayName = *up.DisplayName
	}
	if up.Email != nil {
		nu.Email = *up.Email
	}
	u, err := s.store.CreateUser(r.Context(), nu)
	if errors.Is(err, store.ErrTaken) {
		httpx.Error(w, http.StatusConflict, "username_taken")
		return
	}
	if err != nil {
		s.internalError(w, r, "creating user", err)
		return
	}
	s.audit(r, "user.create", u.Username, string(u.Role))
	_ = s.welcome(r.Context(), u)
	httpx.WriteJSON(w, http.StatusCreated, u)
}

// handleAdminUpdateUser changes profile, role or the disabled flag. The last
// enabled admin can be neither demoted nor disabled.
func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.pathUser(w, r)
	if !ok {
		return
	}
	var body struct {
		DisplayName *string     `json:"displayName"`
		Email       *string     `json:"email"`
		Role        *store.Role `json:"role"`
		Disabled    *bool       `json:"disabled"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 8192); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	up, ok := profileUpdate(w, body.DisplayName, body.Email)
	if !ok {
		return
	}
	if body.Role != nil {
		if *body.Role != store.RoleAdmin && *body.Role != store.RoleUser {
			httpx.Error(w, http.StatusBadRequest, "invalid_role")
			return
		}
		up.Role = body.Role
	}
	up.Disabled = body.Disabled
	demote := (body.Role != nil && *body.Role != store.RoleAdmin) || (body.Disabled != nil && *body.Disabled)
	if demote {
		if last, err := s.lastAdmin(r.Context(), u); err != nil {
			s.internalError(w, r, "counting admins", err)
			return
		} else if last {
			httpx.Error(w, http.StatusConflict, "last_admin")
			return
		}
	}
	nu, err := s.store.UpdateUser(r.Context(), u.ID, up)
	if err != nil {
		s.internalError(w, r, "updating user", err)
		return
	}
	if body.Disabled != nil && *body.Disabled {
		_ = s.store.DeleteUserSessions(r.Context(), u.ID, 0)
	}
	s.audit(r, "user.update", u.Username, describeUpdate(body.Role, body.Disabled))
	httpx.WriteJSON(w, http.StatusOK, nu)
}

func describeUpdate(role *store.Role, disabled *bool) string {
	var parts []string
	if role != nil {
		parts = append(parts, "role="+string(*role))
	}
	if disabled != nil {
		parts = append(parts, "disabled="+strconv.FormatBool(*disabled))
	}
	return strings.Join(parts, " ")
}

// handleAdminSetPassword sets a local account's password and ends its sessions.
func (s *Server) handleAdminSetPassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.pathUser(w, r)
	if !ok {
		return
	}
	if u.Source != store.SourceLocal {
		httpx.Error(w, http.StatusBadRequest, "not_local")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if err := auth.ValidPassword(body.Password); err != nil {
		httpx.Error(w, http.StatusBadRequest, "weak_password")
		return
	}
	hash, err := s.auth.HashNew(body.Password)
	if err != nil {
		s.internalError(w, r, "hashing password", err)
		return
	}
	if _, err := s.store.UpdateUser(r.Context(), u.ID, store.UserUpdate{PasswordHash: &hash}); err != nil {
		s.internalError(w, r, "setting password", err)
		return
	}
	_ = s.store.DeleteUserSessions(r.Context(), u.ID, 0)
	s.audit(r, "user.password", u.Username, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminDeleteUser deletes an account. Its somas go to the account in
// ?transferTo=<id>, or to the trash without it.
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.pathUser(w, r)
	if !ok {
		return
	}
	me, _ := auth.UserFrom(r.Context())
	if u.ID == me.ID {
		httpx.Error(w, http.StatusConflict, "self")
		return
	}
	if last, err := s.lastAdmin(r.Context(), u); err != nil {
		s.internalError(w, r, "counting admins", err)
		return
	} else if last {
		httpx.Error(w, http.StatusConflict, "last_admin")
		return
	}
	var heir store.User
	if v := r.URL.Query().Get("transferTo"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			heir, err = s.store.UserByID(r.Context(), id)
		}
		if err != nil || heir.ID == u.ID {
			httpx.Error(w, http.StatusBadRequest, "invalid_transfer")
			return
		}
	}
	owned, err := s.store.SomasOwnedBy(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, r, "listing somas", err)
		return
	}
	for _, sm := range owned {
		if heir.ID != 0 {
			err = s.store.TransferSoma(r.Context(), sm.ID, heir.ID)
			s.audit(r, "soma.transfer", sm.ID, u.Username+" → "+heir.Username)
		} else {
			err = s.somas.Delete(r.Context(), sm)
			s.audit(r, "soma.delete", sm.ID, sm.Name)
		}
		if err != nil {
			s.internalError(w, r, "handing over somas", err)
			return
		}
	}
	if err := s.store.DeleteUser(r.Context(), u.ID); err != nil {
		s.internalError(w, r, "deleting user", err)
		return
	}
	s.audit(r, "user.delete", u.Username, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"somas": len(owned)})
}

func (s *Server) handleAdminEndUserSessions(w http.ResponseWriter, r *http.Request) {
	u, ok := s.pathUser(w, r)
	if !ok {
		return
	}
	keep := int64(0)
	if me, _ := auth.UserFrom(r.Context()); me.ID == u.ID {
		keep = auth.SessionIDFrom(r.Context()) // never sign yourself out here
	}
	if err := s.store.DeleteUserSessions(r.Context(), u.ID, keep); err != nil {
		s.internalError(w, r, "ending sessions", err)
		return
	}
	s.audit(r, "session.revoke_all", u.Username, "")
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// adminSoma is a soma in the admin list, with its size on disk.
type adminSoma struct {
	store.SomaInfo
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"`
}

func (s *Server) handleAdminSomas(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.AllSomas(r.Context())
	if err != nil {
		s.internalError(w, r, "listing somas", err)
		return
	}
	out := make([]adminSoma, 0, len(list))
	for _, sm := range list {
		p := s.somas.Path(sm.Soma)
		bytes, files := dirUsage(p, 200_000)
		out = append(out, adminSoma{SomaInfo: sm, Path: p, Bytes: bytes, Files: files})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"somas": out})
}

func (s *Server) adminPathSoma(w http.ResponseWriter, r *http.Request) (store.Soma, bool) {
	sm, err := s.store.SomaByID(r.Context(), r.PathValue("soma"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return sm, false
	}
	if err != nil {
		s.internalError(w, r, "soma lookup", err)
		return sm, false
	}
	return sm, true
}

// handleAdminCreateSoma creates a soma for any account, optionally shared with
// a group right away – the admin is no member of somas of others, so this is
// the one place to give such a soma its first group.
func (s *Server) handleAdminCreateSoma(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		OwnerID int64  `json:"ownerId"`
		Group   *struct {
			ID   int64  `json:"id"`
			Role string `json:"role"`
		} `json:"group"`
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
	owner, err := s.store.UserByID(r.Context(), body.OwnerID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && owner.Disabled) {
		httpx.Error(w, http.StatusBadRequest, "unknown_user")
		return
	}
	if err != nil {
		s.internalError(w, r, "user lookup", err)
		return
	}
	var group store.Group
	if body.Group != nil {
		if !store.IsShareRole(body.Group.Role) {
			httpx.Error(w, http.StatusBadRequest, "invalid_role")
			return
		}
		group, err = s.store.GroupByID(r.Context(), body.Group.ID)
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, http.StatusBadRequest, "unknown_group")
			return
		}
		if err != nil {
			s.internalError(w, r, "group lookup", err)
			return
		}
	}
	if owned, err := s.store.SomasOwnedBy(r.Context(), owner.ID); err != nil {
		s.internalError(w, r, "counting somas", err)
		return
	} else if len(owned) >= maxSomasPerUser {
		httpx.Error(w, http.StatusConflict, "too_many_somas")
		return
	}
	sm, err := s.somas.Create(r.Context(), name, owner.ID)
	if err != nil {
		s.internalError(w, r, "creating soma", err)
		return
	}
	info := store.SomaInfo{Soma: sm, Members: 1,
		Owner: store.Person{ID: owner.ID, Username: owner.Username, DisplayName: owner.DisplayName}}
	detail := sm.Name + " for " + owner.Username
	if body.Group != nil {
		if err := s.store.SetSomaGroup(r.Context(), sm.ID, group.ID, body.Group.Role); err != nil {
			s.internalError(w, r, "sharing with group", err)
			return
		}
		info.Groups = 1
		detail += ", shared with " + group.Name + ": " + body.Group.Role
	}
	s.audit(r, "soma.create", sm.ID, detail)
	httpx.WriteJSON(w, http.StatusCreated, info)
}

func (s *Server) handleAdminTransferSoma(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.adminPathSoma(w, r)
	if !ok {
		return
	}
	var body struct {
		OwnerID int64 `json:"ownerId"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	owner, err := s.store.UserByID(r.Context(), body.OwnerID)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_transfer")
		return
	}
	if err := s.store.TransferSoma(r.Context(), sm.ID, owner.ID); err != nil {
		s.internalError(w, r, "transferring soma", err)
		return
	}
	s.audit(r, "soma.transfer", sm.ID, "→ "+owner.Username)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": sm.ID, "ownerId": owner.ID})
}

func (s *Server) handleAdminDeleteSoma(w http.ResponseWriter, r *http.Request) {
	sm, ok := s.adminPathSoma(w, r)
	if !ok {
		return
	}
	if err := s.somas.Delete(r.Context(), sm); err != nil {
		s.internalError(w, r, "deleting soma", err)
		return
	}
	s.audit(r, "soma.delete", sm.ID, sm.Name)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": sm.ID})
}

func (s *Server) handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.Sessions(r.Context())
	if err != nil {
		s.internalError(w, r, "listing sessions", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sessions": list, "current": auth.SessionIDFrom(r.Context())})
}

func (s *Server) handleAdminEndSession(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.store.DeleteSessionByID(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	} else if err != nil {
		s.internalError(w, r, "ending session", err)
		return
	}
	s.audit(r, "session.revoke", strconv.FormatInt(id, 10), "")
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 || limit > 500 {
		limit = 100
	}
	list, err := s.store.AuditLog(r.Context(), before, limit)
	if err != nil {
		s.internalError(w, r, "reading audit log", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": list})
}

// openSoma describes an open soma for the server page.
type openSoma struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Notes   int    `json:"notes"`
	Streams int    `json:"streams"`
	Editing int    `json:"editing"` // notes open for editing together
	Ready   bool   `json:"ready"`
}

// handleAdminServer reports the state of the server and its configuration
// (secrets never included).
func (s *Server) handleAdminServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	open := []openSoma{}
	for _, rt := range s.somas.Running() {
		o := openSoma{ID: rt.ID, Notes: len(rt.Index.Resolver().Notes()), Streams: rt.Streams(), Editing: len(rt.Collab.Open()), Ready: rt.Index.Ready()}
		if sm, err := s.store.SomaByID(ctx, rt.ID); err == nil {
			o.Name = sm.Name
		}
		open = append(open, o)
	}
	users, _ := s.store.Users(ctx)
	somaCount, _ := s.store.CountSomas(ctx)
	sessions, _ := s.store.Sessions(ctx)
	somasBytes, somasFiles := dirUsage(s.cfg.SomasDir, 500_000)
	dataBytes, _ := dirUsage(s.cfg.DataDir, 100_000)

	settings := map[string]string{}
	for _, v := range config.Vars {
		settings[v] = s.cfg.Describe(v)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"version":   s.version,
		"goVersion": runtime.Version(),
		"platform":  runtime.GOOS + "/" + runtime.GOARCH,
		"startedAt": s.started.UTC(),
		"uptime":    time.Since(s.started).Round(time.Second).String(),
		"authMode":  s.cfg.Auth.Mode,
		"counts": map[string]int{
			"users": len(users), "somas": somaCount, "sessions": len(sessions), "openSomas": len(open),
		},
		"open": open,
		"storage": map[string]any{
			"somasDir": s.cfg.SomasDir, "somasBytes": somasBytes, "somasFiles": somasFiles,
			"dataDir": s.cfg.DataDir, "dataBytes": dataBytes,
		},
		"limits": map[string]int64{
			"maxNoteBytes": s.cfg.MaxNoteBytes, "maxUploadBytes": s.cfg.MaxUploadBytes,
		},
		"settings":   settings,
		"localLogin": s.cfg.Auth.LocalLogin,
		"sso":        s.ssoStatus(),
		"ldap":       s.ldapState(),
	})
}

// ssoStatus describes the OpenID Connect provider (nil when not configured).
func (s *Server) ssoStatus() *sso.Status {
	if s.sso == nil {
		return nil
	}
	st := s.sso.Status()
	return &st
}

// handleAdminSSOCheck runs the provider discovery again.
func (s *Server) handleAdminSSOCheck(w http.ResponseWriter, r *http.Request) {
	if s.sso == nil {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	st := s.sso.Check()
	httpx.WriteJSON(w, http.StatusOK, st)
}

// dirUsage adds up the size of the files below dir (at most limit files).
func dirUsage(dir string, limit int) (int64, int) {
	var (
		bytes int64
		files int
	)
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if files >= limit {
			return filepath.SkipAll
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				bytes += info.Size()
				files++
			}
		}
		return nil
	})
	return bytes, files
}
