package server

import (
	"archive/zip"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode"

	"dendronotes/internal/backup"
	"dendronotes/internal/soma"
)

// zipName makes a download file name from a soma's name.
func zipName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`\/:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	if clean == "" {
		clean = "soma"
	}
	return clean + ".zip"
}

// handleExport streams every visible file and folder of a soma as a ZIP
// (hidden entries such as .trash and .obsidian, and ignored names, stay out).
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	sm, _ := somaOf(r)
	rt.Collab.Flush("") // what is being typed right now goes along
	entries, err := rt.Soma.Tree()
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	s.audit(r, "soma.export", sm.ID, sm.Name)
	h := w.Header()
	h.Set("Content-Type", "application/zip")
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": zipName(sm.Name)}))
	h.Set("Cache-Control", "no-store")
	zw := zip.NewWriter(w)
	for _, e := range entries {
		mod := time.UnixMilli(e.MTime)
		if e.Type == "dir" {
			if _, err := zw.CreateHeader(&zip.FileHeader{Name: e.Path + "/", Modified: mod}); err != nil {
				s.log.Warn("export aborted", "soma", sm.ID, "err", err)
				return
			}
			continue
		}
		if err := s.exportFile(zw, rt.Soma, e.Path, mod); err != nil {
			s.log.Warn("export aborted", "soma", sm.ID, "path", e.Path, "err", err)
			return // the client sees a broken download, never a partial one that looks fine
		}
	}
	if err := zw.Close(); err != nil {
		s.log.Warn("export aborted", "soma", sm.ID, "err", err)
	}
}

func (s *Server) exportFile(zw *zip.Writer, sm *soma.Soma, p string, mod time.Time) error {
	f, _, err := sm.OpenFile(p)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: p, Method: zip.Deflate, Modified: mod})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

// handleAdminBackup sends a ZIP with consistent copies of state.db and the
// histories (see internal/backup).
func (s *Server) handleAdminBackup(w http.ResponseWriter, r *http.Request) {
	snap, err := backup.Take(r.Context(), s.store, s.cfg.DataDir)
	if err != nil {
		s.internalError(w, r, "taking a backup", err)
		return
	}
	defer snap.Close()
	s.audit(r, "server.backup", "", strings.Join(snap.Files(), ", "))
	h := w.Header()
	h.Set("Content-Type", "application/zip")
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": snap.Name()}))
	h.Set("Cache-Control", "no-store")
	if err := snap.WriteZip(w, s.version); err != nil {
		s.log.Warn("backup download aborted", "err", err)
	}
}
