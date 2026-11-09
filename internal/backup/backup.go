// Package backup makes a consistent copy of what in DATA_DIR is not a cache:
// state.db (accounts, shares, the trash records …) and the history database
// of every soma. Indexes are left out; they are rebuilt from the somas. The
// notes themselves live in SOMAS_DIR and need a backup of their own.
//
// The copies are made with VACUUM INTO while the server keeps running, then
// packed into a ZIP. To restore, stop dendronotes and unpack the ZIP into
// DATA_DIR.
package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"dendronotes/internal/history"
	"dendronotes/internal/store"
)

// Snapshot holds the copies until they are written out.
type Snapshot struct {
	dir   string
	at    time.Time
	files []string // relative to dir, "/"-separated
}

// Take copies state.db and the histories into a temporary folder inside
// DATA_DIR (the container's root file system is read-only).
func Take(ctx context.Context, st *store.Store, dataDir string) (*Snapshot, error) {
	dir, err := os.MkdirTemp(dataDir, ".backup-")
	if err != nil {
		return nil, err
	}
	s := &Snapshot{dir: dir, at: time.Now().UTC()}
	if err := st.Backup(ctx, filepath.Join(dir, store.FileName)); err != nil {
		s.Close()
		return nil, fmt.Errorf("copy state: %w", err)
	}
	s.files = append(s.files, store.FileName)

	histories, err := filepath.Glob(filepath.Join(dataDir, "history", "*.db"))
	if err != nil {
		s.Close()
		return nil, err
	}
	sort.Strings(histories)
	if len(histories) > 0 {
		if err := os.Mkdir(filepath.Join(dir, "history"), 0o700); err != nil {
			s.Close()
			return nil, err
		}
	}
	for _, f := range histories {
		name := "history/" + filepath.Base(f)
		if err := copyHistory(ctx, f, filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			s.Close()
			return nil, fmt.Errorf("copy %s: %w", name, err)
		}
		s.files = append(s.files, name)
	}
	return s, nil
}

func copyHistory(ctx context.Context, file, to string) error {
	h, err := history.Open(file)
	if err != nil {
		return err
	}
	defer h.Close()
	return h.Backup(ctx, to)
}

// Name is a file name for the ZIP.
func (s *Snapshot) Name() string {
	return "dendronotes-backup-" + s.at.Format("20060102-150405") + ".zip"
}

// Files lists what the backup contains.
func (s *Snapshot) Files() []string { return s.files }

// manifest describes a backup.
type manifest struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	Files     []string  `json:"files"`
	Restore   string    `json:"restore"`
}

// WriteZip writes the backup as a ZIP to w.
func (s *Snapshot) WriteZip(w io.Writer, version string) error {
	zw := zip.NewWriter(w)
	for _, name := range s.files {
		if err := addFile(zw, name, filepath.Join(s.dir, filepath.FromSlash(name)), s.at); err != nil {
			return err
		}
	}
	mw, err := zw.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Deflate, Modified: s.at})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(mw)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest{Version: version, CreatedAt: s.at, Files: s.files,
		Restore: "Stop dendronotes, then unpack these files into DATA_DIR (replacing state.db and history/)."}); err != nil {
		return err
	}
	return zw.Close()
}

func addFile(zw *zip.Writer, name, file string, at time.Time) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: at})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

// Close removes the copies.
func (s *Snapshot) Close() error { return os.RemoveAll(s.dir) }
