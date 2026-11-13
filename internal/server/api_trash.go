package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/events"
	"dendronotes/internal/httpx"
	"dendronotes/internal/soma"
	"dendronotes/internal/somas"
	"dendronotes/internal/store"
)

// The trash of a soma: entries deleted with DELETE …/entries/ wait in the
// soma's .trash folder. Editors see and restore them, owners delete them for
// good.

// trashItem is one deleted file or folder.
type trashItem struct {
	Path      string        `json:"path"` // relative to the trash
	Original  string        `json:"original"`
	Type      string        `json:"type"` // "file" or "dir"
	Size      int64         `json:"size"`
	Files     int           `json:"files,omitempty"` // folders: files inside
	DeletedAt time.Time     `json:"deletedAt"`
	DeletedBy *store.Person `json:"deletedBy"`
}

// recordTrash notes who moved p to the trash (dst is the path in it).
func (s *Server) recordTrash(r *http.Request, somaID, p, dst string, dir bool) {
	me, _ := auth.UserFrom(r.Context())
	rec := store.TrashRecord{
		SomaID: somaID, Path: strings.TrimPrefix(dst, soma.TrashDir+"/"), Original: p, Dir: dir,
		DeletedBy: &store.Person{ID: me.ID},
	}
	if err := s.store.AddTrash(r.Context(), rec); err != nil {
		s.log.Error("recording trash entry", "soma", somaID, "path", p, "err", err)
	}
}

// follow lets what dendronotes keeps about a note or folder – history,
// comments, sort orders and links – go along when it moves (rename, trash,
// restore, or a move outside dendronotes the index recognized).
func (s *Server) follow(rt *somas.Runtime, from, to string) {
	rt.MoveHistory(from, to)
	if err := s.store.MoveComments(context.Background(), rt.ID, from, to); err != nil {
		s.log.Error("moving comments failed", "from", from, "to", to, "err", err)
	}
	if err := s.store.MoveFolderOrder(context.Background(), rt.ID, from, to); err != nil {
		s.log.Error("moving sort orders failed", "from", from, "to", to, "err", err)
	}
	if err := s.store.MoveShareLinks(context.Background(), rt.ID, from, to); err != nil {
		s.log.Error("moving share links failed", "from", from, "to", to, "err", err)
	}
}

// forget deletes what dendronotes keeps about p (deleted for good).
func (s *Server) forget(rt *somas.Runtime, p string) {
	rt.RemoveHistory(p)
	if err := s.store.RemoveComments(context.Background(), rt.ID, p); err != nil {
		s.log.Error("removing comments failed", "path", p, "err", err)
	}
	if err := s.store.RemoveFolderOrder(context.Background(), rt.ID, p); err != nil {
		s.log.Error("removing sort orders failed", "path", p, "err", err)
	}
	if err := s.store.RemoveShareLinks(context.Background(), rt.ID, p); err != nil {
		s.log.Error("removing share links failed", "path", p, "err", err)
	}
}

// trashedFolders are the folders in the trash that were deleted as a whole
// (nothing deleted later may land inside them).
func (s *Server) trashedFolders(r *http.Request, somaID string) (map[string]bool, error) {
	records, err := s.store.TrashRecords(r.Context(), somaID)
	if err != nil {
		return nil, err
	}
	dirs := map[string]bool{}
	for _, rec := range records {
		if rec.Dir {
			dirs[rec.Path] = true
		}
	}
	return dirs, nil
}

// trashItems combines the files in the trash with what is known about them.
// Entries without a record (deleted by Obsidian, or before dendronotes kept
// records) are listed file by file and get a record now: their time in the
// trash counts from when dendronotes first saw them.
func (s *Server) trashItems(ctx context.Context, rt *somas.Runtime) ([]trashItem, error) {
	entries, err := rt.Soma.TrashTree()
	if err != nil {
		return nil, err
	}
	records, err := s.store.TrashRecords(ctx, rt.ID)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]soma.Entry, len(entries))
	for _, e := range entries {
		byPath[e.Path] = e
	}
	under := func(p, dir string) bool { return strings.HasPrefix(p, dir+"/") }

	items := []trashItem{}
	var recordedDirs []string
	recorded := map[string]bool{}
	for _, rec := range records {
		e, ok := byPath[rec.Path]
		if !ok {
			_ = s.store.RemoveTrash(ctx, rt.ID, rec.Path) // gone (purged elsewhere)
			continue
		}
		it := trashItem{Path: rec.Path, Original: rec.Original, Type: e.Type, Size: e.Size, DeletedAt: rec.DeletedAt, DeletedBy: rec.DeletedBy}
		if e.Type == "dir" {
			recordedDirs = append(recordedDirs, rec.Path)
			for _, f := range entries {
				if f.Type == "file" && under(f.Path, rec.Path) {
					it.Size += f.Size
					it.Files++
				}
			}
		}
		recorded[rec.Path] = true
		items = append(items, it)
	}
	for _, e := range entries {
		if e.Type != "file" || recorded[e.Path] || slices.ContainsFunc(recordedDirs, func(d string) bool { return under(e.Path, d) }) {
			continue
		}
		if err := s.store.AddTrash(ctx, store.TrashRecord{SomaID: rt.ID, Path: e.Path, Original: e.Path}); err != nil {
			return nil, err
		}
		items = append(items, trashItem{Path: e.Path, Original: e.Path, Type: "file", Size: e.Size, DeletedAt: time.Now().UTC().Truncate(time.Second)})
	}
	slices.SortStableFunc(items, func(a, b trashItem) int {
		if c := b.DeletedAt.Compare(a.DeletedAt); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	return items, nil
}

func (s *Server) handleListTrash(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	if _, err := s.expireTrash(r.Context(), rt, time.Now()); err != nil {
		s.log.Error("emptying old trash entries failed", "soma", rt.ID, "err", err)
	}
	items, err := s.trashItems(r.Context(), rt)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "keepDays": s.cfg.TrashDays})
}

// expireTrash deletes the entries that are longer than TRASH_DAYS in the
// trash for good (with their history and comments).
func (s *Server) expireTrash(ctx context.Context, rt *somas.Runtime, now time.Time) (int, error) {
	if s.cfg.TrashDays <= 0 {
		return 0, nil
	}
	items, err := s.trashItems(ctx, rt)
	if err != nil {
		return 0, err
	}
	limit := now.AddDate(0, 0, -s.cfg.TrashDays)
	n := 0
	for _, it := range items {
		if !it.DeletedAt.Before(limit) {
			continue
		}
		if err := rt.Soma.PurgeTrash(it.Path); err != nil && !errors.Is(err, soma.ErrNotFound) {
			return n, err
		}
		if err := s.store.RemoveTrash(ctx, rt.ID, it.Path); err != nil {
			return n, err
		}
		s.forget(rt, soma.TrashDir+"/"+it.Path)
		n++
	}
	if n > 0 {
		s.log.Info("old trash entries deleted", "soma", rt.ID, "entries", n, "days", s.cfg.TrashDays)
		s.store.Audit(ctx, store.AuditEntry{Actor: "system", Action: "trash.expire", Target: rt.ID, Detail: strconv.Itoa(n)})
		trashChanged(rt)
	}
	return n, nil
}

// expireTrashEvery empties old trash entries of all somas once an hour
// (somas with an empty trash are not even opened).
func (s *Server) expireTrashEvery(ctx context.Context) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		s.expireAllTrash(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) expireAllTrash(ctx context.Context) {
	list, err := s.store.AllSomas(ctx)
	if err != nil {
		s.log.Error("listing somas for the trash failed", "err", err)
		return
	}
	for _, info := range list {
		if ctx.Err() != nil {
			return
		}
		if entries, err := os.ReadDir(filepath.Join(s.somas.Path(info.Soma), soma.TrashDir)); err != nil || len(entries) == 0 {
			continue
		}
		rt, err := s.somas.Acquire(info.Soma)
		if err != nil {
			continue
		}
		if _, err := s.expireTrash(ctx, rt, time.Now()); err != nil {
			s.log.Error("emptying old trash entries failed", "soma", info.ID, "err", err)
		}
		rt.Release()
	}
}

// handleReadTrash returns a note in the trash (preview).
func (s *Server) handleReadTrash(w http.ResponseWriter, r *http.Request) {
	n, err := runtimeOf(r).Soma.ReadTrashNote(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, n)
}

// trashChanged tells open clients to reload the trash.
func trashChanged(rt *somas.Runtime) {
	rt.Hub.Publish(events.Event{Name: "trash", Data: map[string]string{"soma": rt.ID}})
}

// handleRestoreTrash moves an entry back to where it was deleted from.
func (s *Server) handleRestoreTrash(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	p, err := soma.CleanPath(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	original := p
	records, err := s.store.TrashRecords(r.Context(), rt.ID)
	if err != nil {
		s.internalError(w, r, "reading trash records", err)
		return
	}
	for _, rec := range records {
		if rec.Path == p {
			original = rec.Original
		}
	}
	to, err := rt.Soma.RestoreTrash(p, original)
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	if err := s.store.RemoveTrash(r.Context(), rt.ID, p); err != nil {
		s.log.Error("forgetting trash entry", "soma", rt.ID, "path", p, "err", err)
	}
	s.follow(rt, soma.TrashDir+"/"+p, to)
	rt.SyncAfterWrite(r.Context())
	trashChanged(rt)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"path": to})
}

// handlePurgeTrash deletes an entry in the trash for good (owner).
func (s *Server) handlePurgeTrash(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	p, err := soma.CleanPath(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	if err := rt.Soma.PurgeTrash(p); err != nil {
		s.somaError(w, r, err)
		return
	}
	if err := s.store.RemoveTrash(r.Context(), rt.ID, p); err != nil {
		s.log.Error("forgetting trash entry", "soma", rt.ID, "path", p, "err", err)
	}
	s.forget(rt, soma.TrashDir+"/"+p)
	s.audit(r, "trash.purge", rt.ID, p)
	trashChanged(rt)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"path": p})
}

// handleEmptyTrash deletes everything in the trash for good (owner).
func (s *Server) handleEmptyTrash(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	if err := rt.Soma.EmptyTrash(); err != nil {
		s.somaError(w, r, err)
		return
	}
	if err := s.store.ClearTrash(r.Context(), rt.ID); err != nil {
		s.log.Error("forgetting trash entries", "soma", rt.ID, "err", err)
	}
	s.forget(rt, soma.TrashDir)
	s.audit(r, "trash.empty", rt.ID, "")
	trashChanged(rt)
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Deleted somas (admin area).

func (s *Server) handleAdminDeletedSomas(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.DeletedSomas(r.Context())
	if err != nil {
		s.internalError(w, r, "listing deleted somas", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"somas": list})
}

// handleAdminRestoreSoma brings a deleted soma back; {"ownerId"} chooses
// the owner (required when the old one is gone).
func (s *Server) handleAdminRestoreSoma(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OwnerID int64 `json:"ownerId"`
	}
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
			httpx.WriteDecodeError(w, err)
			return
		}
	}
	d, err := s.store.DeletedSomaByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "reading deleted soma", err)
		return
	}
	ownerID := body.OwnerID
	if ownerID == 0 && d.Owner != nil {
		ownerID = d.Owner.ID
	}
	if ownerID == 0 {
		httpx.Error(w, http.StatusBadRequest, "owner_required")
		return
	}
	owner, err := s.store.UserByID(r.Context(), ownerID)
	if err != nil || owner.Disabled {
		httpx.Error(w, http.StatusBadRequest, "unknown_user")
		return
	}
	sm, err := s.somas.Restore(r.Context(), d.ID, owner.ID)
	switch {
	case errors.Is(err, somas.ErrFolderMissing):
		httpx.Error(w, http.StatusConflict, "folder_missing")
		return
	case err != nil:
		s.internalError(w, r, "restoring soma", err)
		return
	}
	s.audit(r, "soma.restore", sm.ID, sm.Name+" → "+owner.Username)
	httpx.WriteJSON(w, http.StatusOK, sm)
}

// handleAdminPurgeSoma deletes a deleted soma for good.
func (s *Server) handleAdminPurgeSoma(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.DeletedSomaByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "reading deleted soma", err)
		return
	}
	if err := s.somas.Purge(r.Context(), d.ID); err != nil {
		s.internalError(w, r, "purging soma", err)
		return
	}
	s.audit(r, "soma.purge", d.ID, d.Name)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": d.ID})
}
