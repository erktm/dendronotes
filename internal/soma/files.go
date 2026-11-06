package soma

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"strings"
)

// CreateFile stores an uploaded file at p. The content is streamed into a
// temporary file first, so a slow upload never blocks other writes; the
// first bytes go to check (which rejects unwanted types), and more than max
// bytes make it fail with ErrTooLarge. With unique, a taken name gets a
// " (n)" suffix instead of failing with ErrExists. Missing parent folders
// are created, as for notes.
func (s *Soma) CreateFile(p string, r io.Reader, max int64, unique bool, check func(head []byte) error) (Entry, error) {
	p, err := cleanNewPath(p)
	if err != nil {
		return Entry{}, err
	}
	for _, seg := range strings.Split(p, "/") {
		if s.Ignored(seg) {
			return Entry{}, ErrInvalidName
		}
	}

	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return Entry{}, err
	}
	head = head[:n]
	if err := check(head); err != nil {
		return Entry{}, ErrType
	}

	dir := path.Dir(p)
	if dir != "." {
		if err := s.root.MkdirAll(dir, 0o755); err != nil {
			return Entry{}, mapErr(err)
		}
	}
	var rnd [8]byte
	_, _ = rand.Read(rnd[:])
	tmp := path.Join(dir, tmpPrefix+hex.EncodeToString(rnd[:]))
	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Entry{}, mapErr(err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = s.root.Remove(tmp)
		}
	}()

	written, werr := f.Write(head)
	if werr == nil {
		var copied int64
		copied, werr = io.Copy(f, io.LimitReader(r, max-int64(written)+1))
		if werr == nil && int64(written)+copied > max {
			werr = ErrTooLarge
		}
	}
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		if errors.Is(werr, ErrTooLarge) {
			return Entry{}, ErrTooLarge
		}
		return Entry{}, err
	}
	_ = s.root.Chmod(tmp, 0o644) // OpenFile is subject to the umask

	// Only choosing the name and renaming happen under the lock.
	s.mu.Lock()
	defer s.mu.Unlock()
	final := p
	if unique {
		if final, err = s.freeName(p); err != nil {
			return Entry{}, err
		}
	} else if _, err := s.root.Lstat(p); err == nil {
		return Entry{}, ErrExists
	} else if !errors.Is(mapErr(err), ErrNotFound) {
		return Entry{}, mapErr(err)
	}
	if err := s.root.Rename(tmp, final); err != nil {
		return Entry{}, mapErr(err)
	}
	keep = true
	s.syncDir(dir)
	return s.Stat(final)
}

// OpenFile opens a visible file for reading (any type, for serving
// attachments). Symlinks are followed only within the soma.
func (s *Soma) OpenFile(p string) (*os.File, Entry, error) {
	e, err := s.Stat(p)
	if err != nil {
		return nil, Entry{}, err
	}
	if e.Type != "file" {
		return nil, Entry{}, ErrIsDir
	}
	f, err := s.root.Open(e.Path)
	if err != nil {
		return nil, Entry{}, mapErr(err)
	}
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, Entry{}, ErrNotFound
	}
	return f, e, nil
}
