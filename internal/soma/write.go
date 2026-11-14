package soma

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
)

// WriteStage is a moment of a conditional write (WriteNote, WriteFile) at
// which a write hook runs.
type WriteStage int

const (
	// WriteFlushed: the version was checked and the new content written to
	// a temporary file and flushed, the longest part of a write.
	WriteFlushed WriteStage = iota
	// WriteReplacing: the version was checked once more; the new content
	// takes the file's place next.
	WriteReplacing
)

// SetWriteHook makes conditional writes call fn at each WriteStage, so that
// tests can change a file at exactly that moment, as another program might.
// fn runs with the soma locked and must not call it; nil removes the hook.
func (s *Soma) SetWriteHook(fn func(p string, stage WriteStage)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeHook = fn
}

func (s *Soma) hook(p string, stage WriteStage) {
	if s.writeHook != nil {
		s.writeHook(p, stage)
	}
}

// replaceChecked replaces the content of the existing file p with data if it
// still holds the version ifMatch (else a *ConflictError). The data goes into
// a temporary file next to p first, which then takes p's place, so readers
// (and sync tools) never see a half-written file. The caller holds s.mu.
//
// Other programs may change p at any moment, also while data is written and
// flushed. Such a change is not overwritten: p stays as the other program
// left it, or is put back so, and the result is a *ConflictError, as if the
// change had come before the check. Only windows of a system call or two
// remain: a program writing into the replaced file after the last look at
// it, and, where two names cannot be exchanged in one step, a file renamed
// over p between the last look and our rename.
func (s *Soma) replaceChecked(p string, data []byte, ifMatch string) error {
	lst, err := s.root.Lstat(p)
	if err != nil {
		return mapErr(err)
	}
	switch {
	case lst.Mode()&fs.ModeSymlink != 0:
		return ErrReadOnly
	case lst.IsDir():
		return ErrIsDir
	}
	// The checked file stays open: a change written into it later is seen
	// through it, even once p names another file.
	f, err := s.root.Open(p)
	if err != nil {
		return mapErr(err)
	}
	defer f.Close()
	if !s.holds(p, f, ifMatch) {
		return s.conflict(p)
	}
	perm := lst.Mode().Perm()
	tmp, ours, err := s.writeTemp(p, data, perm)
	if err != nil {
		return err
	}
	s.hook(p, WriteFlushed)
	// Flushing takes a while on slow disks: look again before replacing.
	if !s.holds(p, f, ifMatch) {
		_ = s.root.Remove(tmp)
		return s.conflict(p)
	}
	s.hook(p, WriteReplacing)
	return s.swapIn(tmp, ours, p, f, ifMatch, perm)
}

// swapIn puts the temporary file tmp (ours) in p's place. A moment ago p was
// the open file f with the version etag; a change since then is put back.
// Where the system exchanges two names in one step, what p was until then
// lies at tmp afterwards and is looked at. Elsewhere tmp is renamed over p,
// and only a change written into f itself can still be found.
func (s *Soma) swapIn(tmp string, ours fs.FileInfo, p string, f *os.File, etag string, perm fs.FileMode) error {
	dir := path.Dir(p)
	err := s.exchange(tmp, p)
	if errors.Is(err, fs.ErrNotExist) {
		_ = s.root.Remove(tmp)
		return ErrNotFound // removed elsewhere
	}
	if err != nil {
		if err := s.root.Rename(tmp, p); err != nil {
			_ = s.root.Remove(tmp)
			return mapErr(err)
		}
		s.syncDir(dir)
		theirs, err := readAll(f)
		if err != nil || ETag(theirs) == etag {
			return nil
		}
		// Written into the replaced file in the last moment: write it back,
		// unless p was replaced once more meanwhile.
		if st, err := s.root.Lstat(p); err == nil && os.SameFile(st, ours) {
			if back, _, err := s.writeTemp(p, theirs, perm); err == nil {
				if err := s.root.Rename(back, p); err != nil {
					_ = s.root.Remove(back)
				}
				s.syncDir(dir)
			}
		}
		return s.conflict(p)
	}
	if s.holds(tmp, f, etag) {
		_ = s.root.Remove(tmp)
		s.syncDir(dir)
		return nil
	}
	// Changed elsewhere in the last moment: exchange back. If that fails, or
	// p was replaced once more meanwhile, what lies at tmp is the newest
	// change made elsewhere and takes p's place, as it would without us.
	if s.exchange(tmp, p) == nil {
		if st, err := s.root.Lstat(tmp); err == nil && os.SameFile(st, ours) {
			_ = s.root.Remove(tmp)
			s.syncDir(dir)
			return s.conflict(p)
		}
	}
	_ = s.root.Rename(tmp, p)
	s.syncDir(dir)
	return s.conflict(p)
}

// writeTemp writes data to a new temporary file next to p and flushes it.
func (s *Soma) writeTemp(p string, data []byte, perm fs.FileMode) (string, fs.FileInfo, error) {
	var rnd [8]byte
	_, _ = rand.Read(rnd[:])
	tmp := path.Join(path.Dir(p), tmpPrefix+hex.EncodeToString(rnd[:]))

	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return "", nil, mapErr(err)
	}
	_, werr := f.Write(data)
	serr := f.Sync()
	info, ierr := f.Stat()
	cerr := f.Close()
	if err := errors.Join(werr, serr, ierr, cerr); err != nil {
		_ = s.root.Remove(tmp)
		return "", nil, err
	}
	_ = s.root.Chmod(tmp, perm) // OpenFile is subject to the umask
	return tmp, info, nil
}

// holds reports whether name is the open file f and f still has the content
// with the version etag.
func (s *Soma) holds(name string, f *os.File, etag string) bool {
	st, err := s.root.Lstat(name)
	if err != nil {
		return false
	}
	if fi, err := f.Stat(); err != nil || !os.SameFile(st, fi) {
		return false
	}
	data, err := readAll(f)
	return err == nil && ETag(data) == etag
}

// readAll reads all of f from the start, whatever was read from it before.
func readAll(f *os.File) ([]byte, error) {
	return io.ReadAll(io.NewSectionReader(f, 0, math.MaxInt64))
}

// conflict reports a change made elsewhere, with the version now at p.
func (s *Soma) conflict(p string) error {
	data, err := s.root.ReadFile(p)
	if err != nil {
		return mapErr(err)
	}
	return &ConflictError{ETag: ETag(data)}
}

// exchange swaps the entries a and b, both in one folder, in one step. It
// fails where the system or the file system cannot do that.
func (s *Soma) exchange(a, b string) error {
	if s.noExchange {
		return errors.ErrUnsupported
	}
	d, err := s.root.Open(path.Dir(a))
	if err != nil {
		return err
	}
	defer d.Close()
	return exchangeAt(d, path.Base(a), path.Base(b))
}
