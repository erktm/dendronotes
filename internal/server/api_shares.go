package server

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/drawing"
	"dendronotes/internal/httpx"
	"dendronotes/internal/mdparse"
	"dendronotes/internal/soma"
	"dendronotes/internal/somas"
	"dendronotes/internal/store"
)

// Links to notes and drawings follow them when they move (s.follow). An
// internal link (/s/{soma}/l/{id}) opens the entry for everyone with access;
// a public link (/p/{token}) shows it read-only to anyone who has it.

// maxPublicDays is the longest a public link can be set to last.
const maxPublicDays = 365

// inTrash reports whether a soma path lies in the trash.
func inTrash(p string) bool { return p == soma.TrashDir || strings.HasPrefix(p, soma.TrashDir+"/") }

// shareable reports whether an entry can get a public link: notes and drawings.
func shareable(p string) bool { return soma.IsNote(p) || drawing.IsDrawing(p) }

// handleInternalLink returns the internal link of a file, made on first use.
func (s *Server) handleInternalLink(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	me, _ := auth.UserFrom(r.Context())
	var body struct {
		Path string `json:"path"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 8192); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	e, err := rt.Soma.Stat(body.Path)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	if e.Type != "file" {
		httpx.Error(w, http.StatusBadRequest, "not_a_file")
		return
	}
	l, err := s.store.InternalLink(r.Context(), rt.ID, e.Path, me.ID)
	if err != nil {
		s.internalError(w, r, "internal link", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, l)
}

// handleOpenLink says where an internal link points now.
func (s *Server) handleOpenLink(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	l, err := s.store.Link(r.Context(), rt.ID, r.PathValue("id"))
	if err != nil || l.Kind != store.LinkInternal || inTrash(l.Path) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if _, err := rt.Soma.Stat(l.Path); err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, l)
}

// handlePublicLinks lists the soma's public links (editors).
func (s *Server) handlePublicLinks(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	links, err := s.store.PublicLinks(r.Context(), rt.ID)
	if err != nil {
		s.internalError(w, r, "public links", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"links": links})
}

// handleCreatePublicLink makes a public link to a note or drawing. The
// token is in the answer only: dendronotes keeps its SHA-256.
func (s *Server) handleCreatePublicLink(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	me, _ := auth.UserFrom(r.Context())
	var body struct {
		Path string `json:"path"`
		// Days the link lasts; 0 means it does not expire.
		Days int `json:"days"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 8192); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if body.Days < 0 || body.Days > maxPublicDays {
		httpx.Error(w, http.StatusBadRequest, "invalid_days")
		return
	}
	e, err := rt.Soma.Stat(body.Path)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	if e.Type != "file" || !shareable(e.Path) {
		httpx.Error(w, http.StatusBadRequest, "not_shareable")
		return
	}
	var expires time.Time
	if body.Days > 0 {
		expires = time.Now().AddDate(0, 0, body.Days)
	}
	token, l, err := s.store.CreatePublicLink(r.Context(), rt.ID, e.Path, me.ID, expires)
	if errors.Is(err, store.ErrTooManyLinks) {
		httpx.Error(w, http.StatusConflict, "too_many_links")
		return
	}
	if err != nil {
		s.internalError(w, r, "public link", err)
		return
	}
	s.audit(r, "share.public.create", rt.ID, e.Path)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"token": token, "link": l})
}

// handleDeletePublicLink revokes a public link.
func (s *Server) handleDeletePublicLink(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	l, err := s.store.DeletePublicLink(r.Context(), rt.ID, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "revoking public link", err)
		return
	}
	s.audit(r, "share.public.revoke", rt.ID, l.Path)
	w.WriteHeader(http.StatusNoContent)
}

// publicHeaders keep shared pages out of search engines and caches and
// send no referrer to the links in them.
func publicHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Robots-Tag", "noindex, nofollow")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
}

// openPublic finds the entry behind a public token and opens its soma; the
// caller releases the runtime. It answers itself when that fails.
func (s *Server) openPublic(w http.ResponseWriter, r *http.Request) (store.ShareLink, *somas.Runtime, bool) {
	publicHeaders(w)
	if !s.auth.LimitShared(w, r) {
		return store.ShareLink{}, nil, false
	}
	l, err := s.store.PublicLink(r.Context(), r.PathValue("token"))
	if err != nil || inTrash(l.Path) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return store.ShareLink{}, nil, false
	}
	if l.Expired(time.Now()) {
		httpx.Error(w, http.StatusNotFound, "link_expired")
		return store.ShareLink{}, nil, false
	}
	sm, err := s.store.SomaByID(r.Context(), l.SomaID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return store.ShareLink{}, nil, false
	}
	rt, err := s.somas.Acquire(sm)
	if errors.Is(err, somas.ErrClosed) {
		httpx.Error(w, http.StatusServiceUnavailable, "shutting_down")
		return store.ShareLink{}, nil, false
	}
	if err != nil {
		s.internalError(w, r, "opening soma", err)
		return store.ShareLink{}, nil, false
	}
	return l, rt, true
}

// publicFiles are the attachments a shared note refers to: the only files
// its public link may deliver.
func publicFiles(rt *somas.Runtime, notePath, content string) ([]string, error) {
	tree, err := rt.Soma.Tree()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range tree {
		if e.Type == "file" {
			files = append(files, e.Path)
		}
	}
	res := mdparse.NewFileResolver(files)
	out := []string{}
	seen := map[string]bool{}
	for _, ref := range mdparse.Parse(content).Files {
		p := res.Resolve(ref.Target, ref.Markdown, notePath)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// maxPublicDrawing bounds the drawing file a public link delivers.
const maxPublicDrawing = 32 << 20

// handlePublic shows what a public link points to: a note's text with the
// files it may show, or a drawing.
func (s *Server) handlePublic(w http.ResponseWriter, r *http.Request) {
	l, rt, ok := s.openPublic(w, r)
	if !ok {
		return
	}
	defer rt.Release()
	out := map[string]any{"path": l.Path}
	if drawing.IsDrawing(l.Path) {
		f, _, err := rt.Soma.OpenFile(l.Path)
		if err != nil {
			s.somaError(w, r, err)
			return
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxPublicDrawing))
		if err != nil {
			s.internalError(w, r, "reading drawing", err)
			return
		}
		out["kind"], out["svg"] = "drawing", string(data)
	} else {
		n, err := rt.Soma.ReadNote(l.Path)
		if err != nil {
			s.somaError(w, r, err)
			return
		}
		files, err := publicFiles(rt, n.Path, n.Content)
		if err != nil {
			s.internalError(w, r, "listing files", err)
			return
		}
		out["kind"], out["content"], out["files"] = "note", n.Content, files
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// handlePublicFile delivers a file a shared note refers to, and nothing else.
func (s *Server) handlePublicFile(w http.ResponseWriter, r *http.Request) {
	l, rt, ok := s.openPublic(w, r)
	if !ok {
		return
	}
	defer rt.Release()
	if !soma.IsNote(l.Path) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	n, err := rt.Soma.ReadNote(l.Path)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	files, err := publicFiles(rt, n.Path, n.Content)
	if err != nil {
		s.internalError(w, r, "listing files", err)
		return
	}
	want, err := soma.CleanPath(r.PathValue("path"))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	for _, f := range files {
		if f == want {
			s.serveFile(w, r, rt, f)
			return
		}
	}
	httpx.Error(w, http.StatusNotFound, "not_found")
}
