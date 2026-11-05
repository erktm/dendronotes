// Package vault encapsulates every access to the notes directory.
//
// All file operations go through os.Root: the kernel resolves paths relative
// to an open directory handle and refuses any escape, whether via "..",
// absolute paths or symlinks pointing out of the vault. On top of that every
// API path passes CleanPath, which also hides dot entries (.git, .trash …).
package vault

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

// TrashDir is the folder inside the vault that deleted entries are moved to.
const TrashDir = ".trash"

// tmpPrefix marks temporary files of atomic writes (hidden, so the tree and
// the watcher ignore them).
const tmpPrefix = ".leafnotes-tmp-"

// Options configure a vault.
type Options struct {
	Ignore       []string // additional name patterns (path.Match) to hide
	MaxNoteBytes int64    // largest note the editor may open or save
}

// Vault is an opened notes directory.
type Vault struct {
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

// Open opens dir as the vault root.
func Open(dir string, opts Options) (*Vault, error) {
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
		return nil, fmt.Errorf("open vault: %w", err)
	}
	return &Vault{dir: dir, root: root, opts: opts}, nil
}

// Dir returns the absolute path of the vault.
func (v *Vault) Dir() string { return v.dir }

// ErrReplaced means the vault path no longer points to the directory that
// was opened at startup (e.g. the volume was unmounted or replaced).
var ErrReplaced = errors.New("vault directory was replaced or unmounted")

// Check verifies that the vault is still reachable and that its path still
// refers to the same directory.
func (v *Vault) Check() error {
	opened, err := v.root.Stat(".")
	if err != nil {
		return err
	}
	current, err := os.Stat(v.dir)
	if err != nil {
		return err
	}
	if !os.SameFile(opened, current) {
		return ErrReplaced
	}
	return nil
}

// Close releases the directory handle.
func (v *Vault) Close() error { return v.root.Close() }

// Ignored reports whether an entry with this name is hidden from the tree.
func (v *Vault) Ignored(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, p := range v.opts.Ignore {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// Tree lists all visible files and folders.
//
// Symlinks are followed only if they stay inside the vault and point to a
// regular file; such entries are read-only. Symlinked folders are skipped
// (no cycles), escaping symlinks are hidden.
func (v *Vault) Tree() ([]Entry, error) {
	out := []Entry{}
	err := fs.WalkDir(v.root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == "." {
				return err
			}
			return nil // unreadable entry: skip, keep walking
		}
		if p == "." {
			return nil
		}
		if v.Ignored(d.Name()) || validPathOrSkip(p) != nil {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			st, err := v.root.Stat(p) // follows the link, confined to the root
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

func validPathOrSkip(p string) error {
	_, err := CleanPath(p)
	return err
}

// ReadNote returns the content and version of a note.
func (v *Vault) ReadNote(p string) (Note, error) {
	p, err := CleanPath(p)
	if err != nil {
		return Note{}, err
	}
	if !IsNote(p) {
		return Note{}, ErrNotNote
	}
	lst, err := v.root.Lstat(p)
	if err != nil {
		return Note{}, mapErr(err)
	}
	f, err := v.root.Open(p)
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
	if st.Size() > v.opts.MaxNoteBytes {
		return Note{}, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(f, v.opts.MaxNoteBytes+1))
	if err != nil {
		return Note{}, err
	}
	if int64(len(data)) > v.opts.MaxNoteBytes {
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
func (v *Vault) WriteNote(p string, content []byte, ifMatch string) (Note, error) {
	p, err := CleanPath(p)
	if err != nil {
		return Note{}, err
	}
	if !IsNote(p) {
		return Note{}, ErrNotNote
	}
	if int64(len(content)) > v.opts.MaxNoteBytes {
		return Note{}, ErrTooLarge
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	lst, err := v.root.Lstat(p)
	if err != nil {
		return Note{}, mapErr(err)
	}
	switch {
	case lst.Mode()&fs.ModeSymlink != 0:
		return Note{}, ErrReadOnly
	case lst.IsDir():
		return Note{}, ErrIsDir
	}
	current, err := v.root.ReadFile(p)
	if err != nil {
		return Note{}, mapErr(err)
	}
	if cur := ETag(current); ifMatch != cur {
		return Note{}, &ConflictError{ETag: cur}
	}
	if err := v.atomicWrite(p, content, lst.Mode().Perm()); err != nil {
		return Note{}, err
	}
	return v.noteMeta(p, content)
}

// CreateNote creates a new note; missing parent folders are created.
func (v *Vault) CreateNote(p string, content []byte) (Note, error) {
	p, err := cleanNewPath(p)
	if err != nil {
		return Note{}, err
	}
	if !IsNote(p) {
		return Note{}, ErrNotNote
	}
	if int64(len(content)) > v.opts.MaxNoteBytes {
		return Note{}, ErrTooLarge
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if dir := path.Dir(p); dir != "." {
		if err := v.root.MkdirAll(dir, 0o755); err != nil {
			return Note{}, mapErr(err)
		}
	}
	f, err := v.root.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Note{}, mapErr(err)
	}
	_, werr := f.Write(content)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = v.root.Remove(p)
		return Note{}, err
	}
	return v.noteMeta(p, content)
}

// Mkdir creates a folder; missing parent folders are created.
func (v *Vault) Mkdir(p string) (Entry, error) {
	p, err := cleanNewPath(p)
	if err != nil {
		return Entry{}, err
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if dir := path.Dir(p); dir != "." {
		if err := v.root.MkdirAll(dir, 0o755); err != nil {
			return Entry{}, mapErr(err)
		}
	}
	if err := v.root.Mkdir(p, 0o755); err != nil {
		return Entry{}, mapErr(err)
	}
	st, err := v.root.Stat(p)
	if err != nil {
		return Entry{}, mapErr(err)
	}
	return Entry{Path: p, Type: "dir", MTime: st.ModTime().UnixMilli()}, nil
}

// Move renames or moves a file or folder. The target's parent folder must
// exist and the target itself must not (a case-only rename is allowed).
// It returns the cleaned target path.
func (v *Vault) Move(from, to string) (string, error) {
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

	v.mu.Lock()
	defer v.mu.Unlock()

	src, err := v.root.Lstat(from)
	if err != nil {
		return "", mapErr(err)
	}
	if dir := path.Dir(to); dir != "." {
		st, err := v.root.Stat(dir)
		if err != nil {
			return "", mapErr(err)
		}
		if !st.IsDir() {
			return "", ErrNotDir
		}
	}
	if dst, err := v.root.Lstat(to); err == nil {
		if !os.SameFile(src, dst) { // same file = case-only rename on a case-insensitive FS
			return "", ErrExists
		}
	} else if mapped := mapErr(err); mapped != ErrNotFound {
		return "", mapped
	}
	if err := v.root.Rename(from, to); err != nil {
		return "", mapErr(err)
	}
	v.syncDir(path.Dir(to))
	return to, nil
}

// Trash moves a file or folder into .trash/, keeping its relative path.
// Name clashes in the trash get a " (n)" suffix. It returns the trash path.
func (v *Vault) Trash(p string) (string, error) {
	p, err := CleanPath(p)
	if err != nil {
		return "", err
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if _, err := v.root.Lstat(p); err != nil {
		return "", mapErr(err)
	}
	dst := path.Join(TrashDir, p)
	if err := v.root.MkdirAll(path.Dir(dst), 0o755); err != nil {
		return "", mapErr(err)
	}
	dst, err = v.freeName(dst)
	if err != nil {
		return "", err
	}
	if err := v.root.Rename(p, dst); err != nil {
		return "", mapErr(err)
	}
	return dst, nil
}

// freeName returns p, or p with a " (n)" suffix before the extension if p
// already exists.
func (v *Vault) freeName(p string) (string, error) {
	ext := path.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	candidate := p
	for i := 1; i <= 1000; i++ {
		if _, err := v.root.Lstat(candidate); err != nil {
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
func (v *Vault) atomicWrite(p string, data []byte, perm fs.FileMode) error {
	dir := path.Dir(p)
	var rnd [8]byte
	_, _ = rand.Read(rnd[:])
	tmp := path.Join(dir, tmpPrefix+hex.EncodeToString(rnd[:]))

	f, err := v.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return mapErr(err)
	}
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = v.root.Remove(tmp)
		return err
	}
	_ = v.root.Chmod(tmp, perm) // OpenFile is subject to the umask
	if err := v.root.Rename(tmp, p); err != nil {
		_ = v.root.Remove(tmp)
		return mapErr(err)
	}
	v.syncDir(dir)
	return nil
}

// syncDir flushes a directory entry change to disk (best effort; not every
// platform supports fsync on directories).
func (v *Vault) syncDir(dir string) {
	if d, err := v.root.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}

func (v *Vault) noteMeta(p string, content []byte) (Note, error) {
	st, err := v.root.Stat(p)
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
