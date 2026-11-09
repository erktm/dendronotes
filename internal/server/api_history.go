package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"dendronotes/internal/auth"
	"dendronotes/internal/history"
	"dendronotes/internal/httpx"
	"dendronotes/internal/soma"
	"dendronotes/internal/somas"
	"dendronotes/internal/store"
)

// The history of a note: earlier versions kept in the soma's history
// database (internal/history). Everyone with access reads it; writers can
// bring an earlier version back.

// people returns the accounts behind ids (gone accounts are left out).
func (s *Server) people(ctx context.Context, ids []int64) map[int64]store.Person {
	out := map[int64]store.Person{}
	for _, id := range ids {
		if _, ok := out[id]; ok {
			continue
		}
		if u, err := s.store.UserByID(ctx, id); err == nil {
			a := u.Avatar
			out[id] = store.Person{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Avatar: &a}
		}
	}
	return out
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	p, err := soma.CleanPath(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	list, err := rt.History.List(r.Context(), p)
	if err != nil {
		s.internalError(w, r, "listing versions", err)
		return
	}
	var ids []int64
	for _, v := range list {
		ids = append(ids, v.Authors...)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"versions": list, "people": s.people(r.Context(), ids)})
}

// versionOf reads {id} of the soma's history; versions of entries in the
// trash are not reachable here.
func (s *Server) versionOf(w http.ResponseWriter, r *http.Request, rt *somas.Runtime) (history.Version, []byte, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return history.Version{}, nil, false
	}
	v, content, err := rt.History.Get(r.Context(), id)
	if errors.Is(err, history.ErrNotFound) || (err == nil && strings.HasPrefix(v.Path, soma.TrashDir+"/")) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return history.Version{}, nil, false
	}
	if err != nil {
		s.internalError(w, r, "reading a version", err)
		return history.Version{}, nil, false
	}
	return v, content, true
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	v, content, ok := s.versionOf(w, r, runtimeOf(r))
	if !ok {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"version": v, "content": string(content)})
}

// handleRestoreVersion writes an earlier version back to its note; open
// documents take it up like a change made elsewhere.
func (s *Server) handleRestoreVersion(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	v, content, ok := s.versionOf(w, r, rt)
	if !ok {
		return
	}
	rt.Collab.Flush(v.Path) // pending edits first: they become a version of their own
	cur, err := rt.Soma.ReadNote(v.Path)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	if cur.ReadOnly {
		s.somaError(w, r, soma.ErrReadOnly)
		return
	}
	if bytes.Equal([]byte(cur.Content), content) {
		httpx.WriteJSON(w, http.StatusOK, metaOf(cur))
		return
	}
	n, err := rt.Soma.WriteNote(v.Path, content, cur.ETag)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	me, _ := auth.UserFrom(r.Context())
	if err := rt.History.RecordRestore(r.Context(), n.Path, content, []int64{me.ID}, v.ID); err != nil {
		s.log.Error("recording a restore failed", "path", n.Path, "err", err)
	}
	if ch, err := rt.Index.Put(context.WithoutCancel(r.Context()), n); err != nil {
		s.log.Error("index update after restore failed", "path", n.Path, "err", err)
	} else {
		rt.Publish(ch)
	}
	w.Header().Set("ETag", quoteETag(n.ETag))
	httpx.WriteJSON(w, http.StatusOK, metaOf(n))
}
