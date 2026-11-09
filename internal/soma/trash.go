package soma

import (
	"errors"
	"io/fs"
	"path"
	"strings"
)

// The trash keeps deleted entries at their soma-relative path inside
// .trash/ (as Obsidian does). The functions here take paths relative to
// .trash; nothing outside it is ever removed.

// TrashTree lists the files and folders in the trash, with paths relative
// to it. Symlinks are listed as files and never followed.
func (s *Soma) TrashTree() ([]Entry, error) {
	out := []Entry{}
	sub, err := fs.Sub(s.root.FS(), TrashDir)
	if err != nil {
		return nil, err
	}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == "." {
				if errors.Is(err, fs.ErrNotExist) {
					return fs.SkipAll // no trash yet
				}
				return err
			}
			return nil
		}
		if p == "." {
			return nil
		}
		if validPathOrSkip(p) != nil {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		e := Entry{Path: p, Type: "file", Size: info.Size(), MTime: info.ModTime().UnixMilli()}
		if d.IsDir() {
			e.Type, e.Size = "dir", 0
		}
		out = append(out, e)
		return nil
	})
	return out, err
}

// trashPath checks a path relative to the trash and returns it inside the soma.
func trashPath(p string) (string, string, error) {
	p, err := CleanPath(p)
	if err != nil {
		return "", "", err
	}
	return p, path.Join(TrashDir, p), nil
}

// ReadTrashNote reads a note in the trash (for a preview).
func (s *Soma) ReadTrashNote(p string) (Note, error) {
	p, full, err := trashPath(p)
	if err != nil {
		return Note{}, err
	}
	if !IsNote(p) {
		return Note{}, ErrNotNote
	}
	n, err := s.readNote(full)
	n.Path = p
	return n, err
}

// RestoreTrash moves an entry from the trash back to the soma at to (its
// original path), creating missing folders. An occupied target gets a
// " (n)" suffix. It returns where the entry went.
func (s *Soma) RestoreTrash(p, to string) (string, error) {
	_, full, err := trashPath(p)
	if err != nil {
		return "", err
	}
	to, err = CleanPath(to)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.root.Lstat(full); err != nil {
		return "", mapErr(err)
	}
	if dir := path.Dir(to); dir != "." {
		if err := s.root.MkdirAll(dir, 0o755); err != nil {
			return "", mapErr(err)
		}
	}
	to, err = s.freeName(to)
	if err != nil {
		return "", err
	}
	if err := s.root.Rename(full, to); err != nil {
		return "", mapErr(err)
	}
	s.syncDir(path.Dir(to))
	s.pruneTrashDirs(path.Dir(full))
	return to, nil
}

// PurgeTrash deletes an entry in the trash for good.
func (s *Soma) PurgeTrash(p string) error {
	_, full, err := trashPath(p)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.root.Lstat(full); err != nil {
		return mapErr(err)
	}
	if err := s.root.RemoveAll(full); err != nil {
		return mapErr(err)
	}
	s.pruneTrashDirs(path.Dir(full))
	return nil
}

// EmptyTrash deletes everything in the trash for good.
func (s *Soma) EmptyTrash() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.root.RemoveAll(TrashDir); err != nil {
		return mapErr(err)
	}
	return nil
}

// pruneTrashDirs removes folders in the trash that became empty, from dir
// up to (not including) the trash itself.
func (s *Soma) pruneTrashDirs(dir string) {
	for dir != TrashDir && strings.HasPrefix(dir, TrashDir+"/") {
		if s.root.Remove(dir) != nil {
			return // not empty (or gone)
		}
		dir = path.Dir(dir)
	}
}
