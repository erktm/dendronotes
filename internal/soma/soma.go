// Package soma encapsulates every access to the soma, the directory that
// holds the notes (plain Markdown files, the source of truth).
//
// All file operations go through os.Root: the kernel resolves paths relative
// to an open directory handle and refuses any escape, whether via "..",
// absolute paths or symlinks pointing out of the soma. On top of that every
// API path passes CleanPath, which also hides dot entries (.git, .trash …).
package soma

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"unicode/utf8"
)

// TrashDir is the folder inside the soma that deleted entries are moved to.
const TrashDir = ".trash"

// tmpPrefix marks temporary files of atomic writes (hidden, so the tree and
// the watcher ignore them).
const tmpPrefix = ".dendronotes-tmp-"

// Options configure a soma.
type Options struct {
	Ignore       []string // additional name patterns (path.Match) to hide
	MaxNoteBytes int64    // largest note the editor may open or save
}

// Soma is an opened soma directory.
type Soma struct {
	dir  string
	root *os.Root
	opts Options

	// mu serializes mutations so that the ETag check and the write of a
	// note cannot interleave with another request.
	mu sync.Mutex
}

// Entry is a file or folder in the tree.
type Entry struct {
	Path     string `json:"path"`
	Type     string `json:"type"` // "file" or "dir"
	Size     int64  `json:"size,omitempty"`
	MTime    int64  `json:"mtime"` // Unix milliseconds
	ReadOnly bool   `json:"readOnly,omitempty"`
}

// Note is an editable text file with its content and version.
type Note struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	ETag     string `json:"etag"`
	Size     int64  `json:"size"`
	MTime    int64  `json:"mtime"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

// Open opens dir as the soma root.
func Open(dir string, opts Options) (*Soma, error) {
	if opts.MaxNoteBytes <= 0 {
		opts.MaxNoteBytes = 5 << 20
	}
	for _, p := range opts.Ignore {
		if _, err := path.Match(p, ""); err != nil {
			return nil, fmt.Errorf("invalid ignore pattern %q: %w", p, err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open soma: %w", err)
	}
	return &Soma{dir: dir, root: root, opts: opts}, nil
}

// Dir returns the absolute path of the soma.
func (s *Soma) Dir() string { return s.dir }

// ErrReplaced means the soma path no longer points to the directory that
// was opened at startup (e.g. the volume was unmounted or replaced).
var ErrReplaced = errors.New("soma directory was replaced or unmounted")

// Check verifies that the soma is still reachable and that its path still
// refers to the same directory.
func (s *Soma) Check() error {
	opened, err := s.root.Stat(".")
	if err != nil {
		return err
	}
	current, err := os.Stat(s.dir)
	if err != nil {
		return err
	}
	if !os.SameFile(opened, current) {
		return ErrReplaced
	}
	return nil
}

// Close releases the directory handle.
func (s *Soma) Close() error { return s.root.Close() }

// Ignored reports whether an entry with this name is hidden from the tree.
func (s *Soma) Ignored(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, p := range s.opts.Ignore {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// Tree lists all visible files and folders.
//
// Symlinks are followed only if they stay inside the soma and point to a
// regular file; such entries are read-only. Symlinked folders are skipped
// (no cycles), escaping symlinks are hidden.
func (s *Soma) Tree() ([]Entry, error) {
	out := []Entry{}
	err := fs.WalkDir(s.root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == "." {
				return err
			}
			return nil // unreadable entry: skip, keep walking
		}
		if p == "." {
			return nil
		}
		if s.Ignored(d.Name()) || validPathOrSkip(p) != nil {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			st, err := s.root.Stat(p) // follows the link, confined to the root
			if err == nil && st.Mode().IsRegular() {
				out = append(out, Entry{Path: p, Type: "file", Size: st.Size(), MTime: st.ModTime().UnixMilli(), ReadOnly: true})
			}
		case d.IsDir():
			if info, err := d.Info(); err == nil {
				out = append(out, Entry{Path: p, Type: "dir", MTime: info.ModTime().UnixMilli()})
			}
		case d.Type().IsRegular():
			if info, err := d.Info(); err == nil {
				out = append(out, Entry{Path: p, Type: "file", Size: info.Size(), MTime: info.ModTime().UnixMilli()})
			}
		}
		return nil
	})
	return out, err
}

// Stat describes one entry the way Tree lists it. Hidden entries, escaping
// symlinks and symlinked folders are reported as not found.
func (s *Soma) Stat(p string) (Entry, error) {
	p, err := CleanPath(p)
	if err != nil {
		return Entry{}, err
	}
	for _, seg := range strings.Split(p, "/") {
		if s.Ignored(seg) {
			return Entry{}, ErrNotFound
		}
	}
	lst, err := s.root.Lstat(p)
	if err != nil {
		return Entry{}, mapErr(err)
	}
	switch {
	case lst.Mode()&fs.ModeSymlink != 0:
		st, err := s.root.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			return Entry{}, ErrNotFound
		}
		return Entry{Path: p, Type: "file", Size: st.Size(), MTime: st.ModTime().UnixMilli(), ReadOnly: true}, nil
	case lst.IsDir():
		return Entry{Path: p, Type: "dir", MTime: lst.ModTime().UnixMilli()}, nil
	case lst.Mode().IsRegular():
		return Entry{Path: p, Type: "file", Size: lst.Size(), MTime: lst.ModTime().UnixMilli()}, nil
	}
	return Entry{}, ErrNotFound
}

func validPathOrSkip(p string) error {
	_, err := CleanPath(p)
	return err
}

// ReadNote returns the content and version of a note.
func (s *Soma) ReadNote(p string) (Note, error) {
	p, err := CleanPath(p)
	if err != nil {
		return Note{}, err
	}
	if !IsNote(p) {
		return Note{}, ErrNotNote
	}
	lst, err := s.root.Lstat(p)
	if err != nil {
		return Note{}, mapErr(err)
	}
	f, err := s.root.Open(p)
	if err != nil {
		return Note{}, mapErr(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Note{}, err
	}
	if st.IsDir() {
		return Note{}, ErrIsDir
	}
	if st.Size() > s.opts.MaxNoteBytes {
		return Note{}, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(f, s.opts.MaxNoteBytes+1))
	if err != nil {
		return Note{}, err
	}
	if int64(len(data)) > s.opts.MaxNoteBytes {
		return Note{}, ErrTooLarge
	}
	if !utf8.Valid(data) {
		// JSON would silently replace invalid bytes; saving would corrupt the file.
		return Note{}, ErrNotNote
	}
	return Note{
		Path:     p,
		Content:  string(data),
		ETag:     ETag(data),
		Size:     int64(len(data)),
		MTime:    st.ModTime().UnixMilli(),
		ReadOnly: lst.Mode()&fs.ModeSymlink != 0,
	}, nil
}

// WriteNote replaces the content of an existing note. ifMatch must be the
// ETag the client based its changes on; otherwise a *ConflictError with the
// current ETag is returned.
func (s *Soma) WriteNote(p string, content []byte, ifMatch string) (Note, error) {
	p, err := CleanPath(p)
	if err != nil {
		return Note{}, err
	}
	if !IsNote(p) {
		return Note{}, ErrNotNote
	}
	if int64(len(content)) > s.opts.MaxNoteBytes {
		return Note{}, ErrTooLarge
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	lst, err := s.root.Lstat(p)
	if err != nil {
		return Note{}, mapErr(err)
	}
	switch {
	case lst.Mode()&fs.ModeSymlink != 0:
		return Note{}, ErrReadOnly
	case lst.IsDir():
		return Note{}, ErrIsDir
	}
	current, err := s.root.ReadFile(p)
	if err != nil {
		return Note{}, mapErr(err)
	}
	if cur := ETag(current); ifMatch != cur {
		return Note{}, &ConflictError{ETag: cur}
	}
	if err := s.atomicWrite(p, content, lst.Mode().Perm()); err != nil {
		return Note{}, err
	}
	return s.noteMeta(p, content)
}

// CreateNote creates a new note; missing parent folders are created.
func (s *Soma) CreateNote(p string, content []byte) (Note, error) {
	p, err := cleanNewPath(p)
	if err != nil {
		return Note{}, err
	}
	if !IsNote(p) {
		return Note{}, ErrNotNote
	}
	if int64(len(content)) > s.opts.MaxNoteBytes {
		return Note{}, ErrTooLarge
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if dir := path.Dir(p); dir != "." {
		if err := s.root.MkdirAll(dir, 0o755); err != nil {
			return Note{}, mapErr(err)
		}
	}
	f, err := s.root.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Note{}, mapErr(err)
	}
	_, werr := f.Write(content)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = s.root.Remove(p)
		return Note{}, err
	}
	return s.noteMeta(p, content)
}

// Mkdir creates a folder; missing parent folders are created.
func (s *Soma) Mkdir(p string) (Entry, error) {
	p, err := cleanNewPath(p)
	if err != nil {
		return Entry{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if dir := path.Dir(p); dir != "." {
		if err := s.root.MkdirAll(dir, 0o755); err != nil {
			return Entry{}, mapErr(err)
		}
	}
	if err := s.root.Mkdir(p, 0o755); err != nil {
		return Entry{}, mapErr(err)
	}
	st, err := s.root.Stat(p)
	if err != nil {
		return Entry{}, mapErr(err)
	}
	return Entry{Path: p, Type: "dir", MTime: st.ModTime().UnixMilli()}, nil
}

// Move renames or moves a file or folder. The target's parent folder must
// exist and the target itself must not (a case-only rename is allowed).
// It returns the cleaned target path.
func (s *Soma) Move(from, to string) (string, error) {
	from, err := CleanPath(from)
	if err != nil {
		return "", err
	}
	to, err = cleanNewPath(to)
	if err != nil {
		return "", err
	}
	if from == to {
		return to, nil
	}
	if strings.HasPrefix(to, from+"/") {
		return "", ErrMoveIntoSelf
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	src, err := s.root.Lstat(from)
	if err != nil {
		return "", mapErr(err)
	}
	if dir := path.Dir(to); dir != "." {
		st, err := s.root.Stat(dir)
		if err != nil {
			return "", mapErr(err)
		}
		if !st.IsDir() {
			return "", ErrNotDir
		}
	}
	if dst, err := s.root.Lstat(to); err == nil {
		if !os.SameFile(src, dst) { // same file = case-only rename on a case-insensitive FS
			return "", ErrExists
		}
	} else if mapped := mapErr(err); mapped != ErrNotFound {
		return "", mapped
	}
	if err := s.root.Rename(from, to); err != nil {
		return "", mapErr(err)
	}
	s.syncDir(path.Dir(to))
	return to, nil
}

// Trash moves a file or folder into .trash/, keeping its relative path.
// Name clashes in the trash get a " (n)" suffix. It returns the trash path.
func (s *Soma) Trash(p string) (string, error) {
	p, err := CleanPath(p)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.root.Lstat(p); err != nil {
		return "", mapErr(err)
	}
	dst := path.Join(TrashDir, p)
	if err := s.root.MkdirAll(path.Dir(dst), 0o755); err != nil {
		return "", mapErr(err)
	}
	dst, err = s.freeName(dst)
	if err != nil {
		return "", err
	}
	if err := s.root.Rename(p, dst); err != nil {
		return "", mapErr(err)
	}
	return dst, nil
}

// freeName returns p, or p with a " (n)" suffix before the extension if p
// already exists.
func (s *Soma) freeName(p string) (string, error) {
	ext := path.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	candidate := p
	for i := 1; i <= 1000; i++ {
		if _, err := s.root.Lstat(candidate); err != nil {
			if mapErr(err) == ErrNotFound {
				return candidate, nil
			}
			return "", mapErr(err)
		}
		candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
	return "", ErrExists
}

// atomicWrite writes data to a temporary file next to p and renames it over
// p, so readers (and sync tools) never see a half-written note.
func (s *Soma) atomicWrite(p string, data []byte, perm fs.FileMode) error {
	dir := path.Dir(p)
	var rnd [8]byte
	_, _ = rand.Read(rnd[:])
	tmp := path.Join(dir, tmpPrefix+hex.EncodeToString(rnd[:]))

	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return mapErr(err)
	}
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = s.root.Remove(tmp)
		return err
	}
	_ = s.root.Chmod(tmp, perm) // OpenFile is subject to the umask
	if err := s.root.Rename(tmp, p); err != nil {
		_ = s.root.Remove(tmp)
		return mapErr(err)
	}
	s.syncDir(dir)
	return nil
}

// syncDir flushes a directory entry change to disk (best effort; not every
// platform supports fsync on directories).
func (s *Soma) syncDir(dir string) {
	if d, err := s.root.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}

func (s *Soma) noteMeta(p string, content []byte) (Note, error) {
	st, err := s.root.Stat(p)
	if err != nil {
		return Note{}, mapErr(err)
	}
	return Note{
		Path:    p,
		Content: string(content),
		ETag:    ETag(content),
		Size:    int64(len(content)),
		MTime:   st.ModTime().UnixMilli(),
	}, nil
}

// ETag returns the version identifier of a note's content.
func ETag(data []byte) string {
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:18])
}
