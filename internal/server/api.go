package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"dendronotes/internal/auth"
	"dendronotes/internal/history"
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
		{"GET", "/api/auth/methods", s.auth.HandleMethods, public},
		{"POST", "/api/auth/password/forgot", s.auth.HandleForgotPassword, public},
		{"POST", "/api/auth/password/reset", s.auth.HandleResetPassword, public},
		{"GET", "/api/auth/banners", s.handleLoginBanners, public},
		{"GET", "/api/me/banners", s.handleMyBanners, signedIn},
		{"POST", "/api/me/banners/{id}/dismiss", s.handleDismissBanner, signedIn},
		{"GET", "/api/auth/oidc/login", s.auth.HandleSSOLogin, public},
		{"GET", "/api/auth/oidc/callback", s.auth.HandleSSOCallback, public},

		{"PATCH", "/api/me", s.handleUpdateProfile, signedIn},
		{"POST", "/api/me/password", s.handleChangePassword, signedIn},
		{"PUT", "/api/me/avatar", s.handleSetAvatar, signedIn},
		{"PUT", "/api/me/avatar/photo", s.handleSetAvatarPhoto, signedIn},
		{"DELETE", "/api/me/avatar/photo", s.handleRemoveAvatarPhoto, signedIn},
		{"GET", "/api/users/{id}/avatar", s.handleAvatarPhoto, signedIn},

		{"GET", "/api/somas", s.handleListSomas, signedIn},
		{"POST", "/api/somas", s.handleCreateSoma, signedIn},
		{"PATCH", "/api/somas/{soma}", s.handleUpdateSoma, somaOwner},
		{"DELETE", "/api/somas/{soma}", s.handleDeleteSoma, somaOwner},
		{"GET", "/api/somas/{soma}/members", s.handleMembers, somaViewer},
		{"PUT", "/api/somas/{soma}/members/{id}", s.handleSetMember, somaOwner},
		{"DELETE", "/api/somas/{soma}/members/{id}", s.handleRemoveMember, somaOwner},
		{"PUT", "/api/somas/{soma}/groups/{id}", s.handleSetSomaGroup, somaOwner},
		{"DELETE", "/api/somas/{soma}/groups/{id}", s.handleRemoveSomaGroup, somaOwner},
		{"DELETE", "/api/somas/{soma}/membership", s.handleLeaveSoma, somaViewer},
		{"GET", "/api/users/search", s.handleSearchUsers, signedIn},
		{"GET", "/api/me/notifications", s.handleNotifications, signedIn},
		{"GET", "/api/me/tokens", s.handleTokens, signedIn},
		{"POST", "/api/me/tokens", s.handleCreateToken, signedIn},
		{"DELETE", "/api/me/tokens/{id}", s.handleDeleteToken, signedIn},
		{"POST", "/api/me/notifications/read", s.handleReadNotifications, signedIn},

		{"GET", "/api/somas/{soma}/tree", s.handleTree, somaViewer},
		{"GET", "/api/somas/{soma}/notes/{path...}", s.handleReadNote, somaViewer},
		{"PUT", "/api/somas/{soma}/notes/{path...}", s.handleWriteNote, somaWriter},
		{"POST", "/api/somas/{soma}/notes/{path...}", s.handleCreateNote, somaContributor},
		{"POST", "/api/somas/{soma}/folders/{path...}", s.handleMkdir, somaContributor},
		{"GET", "/api/somas/{soma}/files/{path...}", s.handleReadFile, somaViewer},
		{"POST", "/api/somas/{soma}/files/{path...}", s.handleUpload, somaContributor},
		{"GET", "/api/somas/{soma}/drawings/{path...}", s.handleReadDrawing, somaViewer},
		{"POST", "/api/somas/{soma}/drawings/{path...}", s.handleCreateDrawing, somaContributor},
		{"PUT", "/api/somas/{soma}/drawings/{path...}", s.handleSaveDrawing, somaWriter},
		{"GET", "/api/somas/{soma}/icons", s.handleIcons, somaViewer},
		{"POST", "/api/somas/{soma}/icons", s.handleAddIcon, somaContributor},
		{"GET", "/api/somas/{soma}/icons/{id}", s.handleIcon, somaViewer},
		{"DELETE", "/api/somas/{soma}/icons/{id}", s.handleDeleteIcon, somaViewer},
		{"POST", "/api/somas/{soma}/move", s.handleMove, somaEditor},
		{"DELETE", "/api/somas/{soma}/entries/{path...}", s.handleTrash, somaEditor},
		{"GET", "/api/somas/{soma}/export", s.handleExport, somaViewer},
		{"GET", "/api/somas/{soma}/comments", s.handleComments, somaViewer},
		{"POST", "/api/somas/{soma}/comments", s.handleAddComment, somaViewer},
		{"PATCH", "/api/somas/{soma}/comments/{id}", s.handleEditComment, somaViewer},
		{"DELETE", "/api/somas/{soma}/comments/{id}", s.handleDeleteComment, somaViewer},
		{"POST", "/api/somas/{soma}/comments/{id}/resolve", s.handleResolveComment, somaWriter},
		{"GET", "/api/somas/{soma}/people", s.handleSomaPeople, somaViewer},
		{"GET", "/api/somas/{soma}/history/{path...}", s.handleHistory, somaViewer},
		{"GET", "/api/somas/{soma}/versions/{id}", s.handleVersion, somaViewer},
		{"POST", "/api/somas/{soma}/versions/{id}/restore", s.handleRestoreVersion, somaWriter},
		{"GET", "/api/somas/{soma}/trash", s.handleListTrash, somaEditor},
		{"DELETE", "/api/somas/{soma}/trash", s.handleEmptyTrash, somaOwner},
		{"GET", "/api/somas/{soma}/trash/{path...}", s.handleReadTrash, somaEditor},
		{"POST", "/api/somas/{soma}/trash/{path...}", s.handleRestoreTrash, somaEditor},
		{"DELETE", "/api/somas/{soma}/trash/{path...}", s.handlePurgeTrash, somaOwner},
		{"GET", "/api/somas/{soma}/search", s.handleSearch, somaViewer},
		{"GET", "/api/somas/{soma}/backlinks/{path...}", s.handleBacklinks, somaViewer},
		{"GET", "/api/somas/{soma}/tags", s.handleTags, somaViewer},
		{"GET", "/api/somas/{soma}/graph", s.handleGraph, somaViewer},
		{"GET", "/api/somas/{soma}/events", s.handleEvents, somaViewer},
		{"POST", "/api/somas/{soma}/collab/join", s.handleCollabJoin, somaViewer},
		{"POST", "/api/somas/{soma}/collab/leave", s.handleCollabLeave, somaViewer},
		{"POST", "/api/somas/{soma}/collab/push", s.handleCollabPush, somaWriter},
		{"GET", "/api/somas/{soma}/collab/pull", s.handleCollabPull, somaViewer},
		{"POST", "/api/somas/{soma}/collab/presence", s.handleCollabPresence, somaViewer},

		{"GET", "/api/admin/users", s.handleAdminUsers, adminOnly},
		{"POST", "/api/admin/users", s.handleAdminCreateUser, adminOnly},
		{"PATCH", "/api/admin/users/{id}", s.handleAdminUpdateUser, adminOnly},
		{"POST", "/api/admin/users/{id}/password", s.handleAdminSetPassword, adminOnly},
		{"DELETE", "/api/admin/users/{id}", s.handleAdminDeleteUser, adminOnly},
		{"DELETE", "/api/admin/users/{id}/sessions", s.handleAdminEndUserSessions, adminOnly},
		{"DELETE", "/api/admin/users/{id}/tokens", s.handleAdminRevokeTokens, adminOnly},
		{"GET", "/api/admin/groups", s.handleAdminGroups, adminOnly},
		{"POST", "/api/admin/groups", s.handleAdminCreateGroup, adminOnly},
		{"PATCH", "/api/admin/groups/{id}", s.handleAdminRenameGroup, adminOnly},
		{"DELETE", "/api/admin/groups/{id}", s.handleAdminDeleteGroup, adminOnly},
		{"GET", "/api/admin/groups/{id}/members", s.handleAdminGroupMembers, adminOnly},
		{"PUT", "/api/admin/groups/{id}/members/{user}", s.handleAdminAddGroupMember, adminOnly},
		{"DELETE", "/api/admin/groups/{id}/members/{user}", s.handleAdminRemoveGroupMember, adminOnly},
		{"GET", "/api/admin/somas", s.handleAdminSomas, adminOnly},
		{"POST", "/api/admin/somas", s.handleAdminCreateSoma, adminOnly},
		{"PATCH", "/api/admin/somas/{soma}", s.handleAdminTransferSoma, adminOnly},
		{"DELETE", "/api/admin/somas/{soma}", s.handleAdminDeleteSoma, adminOnly},
		{"GET", "/api/admin/somas/deleted", s.handleAdminDeletedSomas, adminOnly},
		{"POST", "/api/admin/somas/deleted/{id}/restore", s.handleAdminRestoreSoma, adminOnly},
		{"DELETE", "/api/admin/somas/deleted/{id}", s.handleAdminPurgeSoma, adminOnly},
		{"GET", "/api/admin/sessions", s.handleAdminSessions, adminOnly},
		{"DELETE", "/api/admin/sessions/{id}", s.handleAdminEndSession, adminOnly},
		{"GET", "/api/admin/server", s.handleAdminServer, adminOnly},
		{"POST", "/api/admin/mail/test", s.handleAdminTestMail, adminOnly},
		{"GET", "/api/admin/banners", s.handleAdminBanners, adminOnly},
		{"POST", "/api/admin/banners", s.handleAdminCreateBanner, adminOnly},
		{"PATCH", "/api/admin/banners/{id}", s.handleAdminUpdateBanner, adminOnly},
		{"DELETE", "/api/admin/banners/{id}", s.handleAdminDeleteBanner, adminOnly},
		{"POST", "/api/admin/sso/check", s.handleAdminSSOCheck, adminOnly},
		{"POST", "/api/admin/ldap/check", s.handleAdminLDAPCheck, adminOnly},
		{"POST", "/api/admin/ldap/sync", s.handleAdminLDAPSync, adminOnly},
		{"GET", "/api/admin/backup", s.handleAdminBackup, adminOnly},
		{"GET", "/api/admin/audit", s.handleAdminAudit, adminOnly},
	}
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	for _, rt := range s.routes() {
		var h http.Handler = rt.handler
		switch rt.access {
		case public:
		case signedIn:
			h = s.auth.Require(s.tokenGuard(rt, h))
		case adminOnly:
			h = s.auth.RequireAdmin(s.tokenGuard(rt, h))
		default:
			h = s.auth.Require(s.tokenGuard(rt, s.withSoma(rt.access.somaRole(), h)))
		}
		mux.Handle(rt.method+" "+rt.pattern, labelRoute(rt.method+" "+rt.pattern, h))
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
	me, _ := auth.UserFrom(r.Context())
	rt.RecordVersion(n.Path, []byte(*body.Content), []int64{me.ID}, history.ReasonEdit)
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
	me, _ := auth.UserFrom(r.Context())
	rt.RecordVersion(n.Path, []byte(body.Content), []int64{me.ID}, history.ReasonCreate)
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
	me, _ := auth.UserFrom(r.Context())
	var body struct {
		From           string `json:"from"`
		To             string `json:"to"`
		UpdateSynapses bool   `json:"updateSynapses"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 16<<10); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	from, err := soma.CleanPath(body.From)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	// Open documents are saved first and follow the move.
	move := func() (string, error) {
		to, err := rt.Collab.Move(from, func() (string, error) { return rt.Soma.Move(from, body.To) })
		if err == nil {
			s.follow(rt, from, to)
		}
		return to, err
	}
	if !body.UpdateSynapses {
		to, err := move()
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
	rt.Collab.Flush("") // the rewrite reads what is on disk
	edits, err := s.planRelink(r, rt, from, body.To)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	to, err := move()
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
		rt.RecordVersion(e.Path, []byte(e.Content), []int64{me.ID}, history.ReasonRelink)
		res.Notes++
		res.Synapses += e.Synapses
		res.Embeds += e.Embeds
	}
	rt.SyncAfterWrite(r.Context())
	httpx.WriteJSON(w, http.StatusOK, res)
}

// moveResult reports a move and the synapses rewritten for it.
type moveResult struct {
	Path     string   `json:"path"`
	Notes    int      `json:"updatedNotes"`
	Synapses int      `json:"updatedSynapses"`
	Embeds   int      `json:"updatedEmbeds"`
	Failed   []string `json:"failed"`
}

// planRelink computes the rewrites of synapses and attachment references
// for moving from to to: a note, an attachment or a folder with everything
// inside it.
func (s *Server) planRelink(r *http.Request, rt *somas.Runtime, from, to string) ([]relink.Edit, error) {
	from, err := soma.CleanPath(from)
	if err != nil {
		return nil, err
	}
	to, err = soma.CleanPath(to)
	if err != nil {
		return nil, err
	}
	entries, err := rt.Soma.Tree()
	if err != nil {
		return nil, err
	}
	notes := rt.Index.Resolver().Notes()
	var files, movedNotes, movedFiles []string
	moves := map[string]string{}
	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		files = append(files, e.Path)
		switch {
		case e.Path == from:
			moves[e.Path] = to
		case strings.HasPrefix(e.Path, from+"/"):
			moves[e.Path] = to + e.Path[len(from):]
		default:
			continue
		}
		if soma.IsNote(e.Path) {
			movedNotes = append(movedNotes, e.Path)
		} else {
			movedFiles = append(movedFiles, e.Path)
		}
	}
	if len(moves) == 0 {
		return nil, nil // an empty folder
	}
	sources, err := rt.Index.Referrers(r.Context(), movedNotes)
	if err != nil {
		return nil, err
	}
	fileSources, err := rt.Index.FileReferrers(r.Context(), movedFiles)
	if err != nil {
		return nil, err
	}
	read := func(p string) (string, string, error) {
		n, err := rt.Soma.ReadNote(p)
		return n.Content, n.ETag, err
	}
	return relink.Plan(read, append(sources, fileSources...), notes, files, moves)
}

func (s *Server) handleTrash(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	p, err := soma.CleanPath(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	entry, _ := rt.Soma.Stat(p) // only to remember whether it was a folder
	deletedDirs, err := s.trashedFolders(r, rt.ID)
	if err != nil {
		s.internalError(w, r, "reading trash records", err)
		return
	}
	// Open documents are saved before they go to the trash, then closed.
	var dst string
	err = rt.Collab.Remove(p, func() (err error) {
		dst, err = rt.Soma.TrashAvoiding(p, func(d string) bool { return deletedDirs[d] })
		return err
	})
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	s.recordTrash(r, rt.ID, p, dst, entry.Type == "dir")
	s.follow(rt, p, dst)
	trashChanged(rt)
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
