package server

import (
	"bytes"
	"errors"
	"net/http"
	"path"
	"strings"

	"dendronotes/internal/attach"
	"dendronotes/internal/auth"
	"dendronotes/internal/drawing"
	"dendronotes/internal/history"
	"dendronotes/internal/httpx"
	"dendronotes/internal/soma"
	"dendronotes/internal/somas"
	"dendronotes/internal/store"
)

// A copy is bounded like an export: at most this many entries and bytes.
const (
	maxCopyEntries = 5000
	maxCopyBytes   = 512 << 20
)

// copyResult reports a copy: where it landed and what was left out.
type copyResult struct {
	Path    string     `json:"path"`
	Files   int        `json:"files"`
	Folders int        `json:"folders"`
	Skipped []copySkip `json:"skipped"`
}

type copySkip struct {
	File   string `json:"file"`
	Reason string `json:"reason"`
}

// handleCopy copies a note, drawing, file or folder of this soma into a
// folder of another soma, or of this one, where the caller may create
// entries (contributor). A taken name gets a " (n)" suffix; inside a copied
// folder everything keeps its name. Notes start a history of their own;
// comments, links and the history of the originals stay behind. Files the
// target would not accept as uploads are left out and listed.
func (s *Server) handleCopy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	src := runtimeOf(r)
	srcSoma, _ := somaOf(r)
	me, _ := auth.UserFrom(ctx)
	var body struct {
		From string `json:"from"`
		Soma string `json:"soma"`
		To   string `json:"to"`
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
	entry, err := src.Soma.Stat(from)
	if err != nil {
		s.somaError(w, r, err)
		return
	}

	// The target: the same rules as withSoma, at the contributor level.
	role, err := s.store.EffectiveRole(ctx, body.Soma, me.ID)
	if err != nil {
		s.internalError(w, r, "membership lookup", err)
		return
	}
	if role == "" {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if store.RoleRank(role) < store.RoleRank(store.MemberContributor) {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	dst := src
	if body.Soma != srcSoma.ID {
		sm, err := s.store.SomaByID(ctx, body.Soma)
		if errors.Is(err, store.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found")
			return
		}
		if err != nil {
			s.internalError(w, r, "soma lookup", err)
			return
		}
		if dst, err = s.somas.Acquire(sm); errors.Is(err, somas.ErrClosed) {
			httpx.Error(w, http.StatusServiceUnavailable, "shutting_down")
			return
		} else if err != nil {
			s.internalError(w, r, "opening soma", err)
			return
		}
		defer dst.Release()
	}
	to := ""
	if strings.Trim(body.To, "/") != "" {
		folder, err := dst.Soma.Stat(body.To)
		if err != nil {
			s.somaError(w, r, err)
			return
		}
		if folder.Type != "dir" {
			httpx.Error(w, http.StatusBadRequest, "not_a_directory")
			return
		}
		to = folder.Path
	}

	// What to copy, fixed before anything is written (a folder copied into
	// itself does not copy its copy). Unsaved edits count.
	src.Collab.Flush(from)
	plan := []soma.Entry{entry}
	if entry.Type == "dir" {
		all, err := src.Soma.Tree()
		if err != nil {
			s.internalError(w, r, "listing soma", err)
			return
		}
		for _, e := range all {
			if strings.HasPrefix(e.Path, from+"/") {
				plan = append(plan, e)
			}
		}
	}
	var size int64
	for _, e := range plan {
		size += e.Size
	}
	if len(plan) > maxCopyEntries || size > maxCopyBytes {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}

	res := copyResult{Skipped: []copySkip{}}
	top := path.Join(to, path.Base(from))
	if entry.Type == "dir" {
		made, err := dst.Soma.MkdirUnique(top)
		if err != nil {
			s.somaError(w, r, err)
			return
		}
		res.Path, res.Folders = made.Path, 1
		for _, e := range plan[1:] {
			target := path.Join(made.Path, strings.TrimPrefix(e.Path, from+"/"))
			if e.Type == "dir" {
				if _, err := dst.Soma.Mkdir(target); err != nil && !errors.Is(err, soma.ErrExists) {
					res.Skipped = append(res.Skipped, copySkip{File: e.Path, Reason: "the folder could not be made"})
					continue
				}
				res.Folders++
				continue
			}
			if _, reason := s.copyFile(src, dst, e, target, false, me.ID); reason != "" {
				res.Skipped = append(res.Skipped, copySkip{File: e.Path, Reason: reason})
				continue
			}
			res.Files++
		}
	} else {
		landed, reason := s.copyFile(src, dst, entry, top, true, me.ID)
		if reason != "" {
			httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "not_copied", "reason": reason})
			return
		}
		res.Path, res.Files = landed, 1
	}
	dst.SyncAfterWrite(ctx)
	httpx.WriteJSON(w, http.StatusCreated, res)
}

// copyFile copies one file to target in dst, as a new note, drawing or
// upload; it returns where it landed, or why it was left out.
func (s *Server) copyFile(src, dst *somas.Runtime, e soma.Entry, target string, unique bool, by int64) (string, string) {
	switch {
	case soma.IsNote(e.Path):
		n, err := src.Soma.ReadNote(e.Path)
		if errors.Is(err, soma.ErrTooLarge) {
			return "", "too large to open as a note"
		}
		if err != nil {
			return "", "the note could not be read"
		}
		create := dst.Soma.CreateNote
		if unique {
			create = dst.Soma.CreateNoteUnique
		}
		made, err := create(target, []byte(n.Content))
		if err != nil {
			return "", reasonOf(err)
		}
		dst.RecordVersion(made.Path, []byte(n.Content), []int64{by}, history.ReasonCreate)
		return made.Path, ""
	case drawing.IsDrawing(e.Path):
		data, _, _, err := src.Soma.ReadFileContent(e.Path, drawing.MaxBytes)
		if err != nil {
			return "", reasonOf(err)
		}
		if drawing.Check(data) != nil {
			return "", "not a drawing dendronotes can open"
		}
		made, err := dst.Soma.CreateFile(target, bytes.NewReader(data), drawing.MaxBytes, unique, func([]byte) error { return nil })
		if err != nil {
			return "", reasonOf(err)
		}
		return made.Path, ""
	default:
		f, _, err := src.Soma.OpenFile(e.Path)
		if err != nil {
			return "", reasonOf(err)
		}
		defer f.Close()
		made, err := dst.Soma.CreateFile(target, f, s.cfg.MaxUploadBytes, unique, func(head []byte) error {
			return attach.Check(target, head)
		})
		if err != nil {
			return "", reasonOf(err)
		}
		return made.Path, ""
	}
}

// reasonOf says in a few words why a file was not copied.
func reasonOf(err error) string {
	switch {
	case errors.Is(err, soma.ErrType):
		return "a file type the soma does not accept"
	case errors.Is(err, soma.ErrTooLarge):
		return "too large"
	case errors.Is(err, soma.ErrExists):
		return "the name is taken"
	case errors.Is(err, soma.ErrInvalidName), errors.Is(err, soma.ErrInvalidPath):
		return "a name the soma does not accept"
	}
	return "the file could not be copied"
}
