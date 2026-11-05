package vault

import (
	"errors"
	"io/fs"
	"strings"
	"syscall"
)

// Errors returned by the vault. The HTTP layer maps them to status codes.
var (
	ErrInvalidPath  = errors.New("invalid path")
	ErrInvalidName  = errors.New("invalid name")
	ErrNotFound     = errors.New("not found")
	ErrExists       = errors.New("already exists")
	ErrConflict     = errors.New("changed on disk")
	ErrNotNote      = errors.New("not an editable note")
	ErrReadOnly     = errors.New("read-only")
	ErrTooLarge     = errors.New("too large")
	ErrNotDir       = errors.New("not a directory")
	ErrIsDir        = errors.New("is a directory")
	ErrMoveIntoSelf = errors.New("cannot move a folder into itself")
)

// ConflictError is returned when a write is based on an outdated version.
// ETag is the version currently on disk.
type ConflictError struct{ ETag string }

func (e *ConflictError) Error() string        { return ErrConflict.Error() }
func (e *ConflictError) Is(target error) bool { return target == ErrConflict }

// mapErr translates file system errors into vault errors. Attempts to leave
// the vault (via "..", absolute or escaping symlinks) are reported as
// ErrNotFound so that nothing about the outside is revealed.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist),
		errors.Is(err, syscall.ENOTDIR),
		errors.Is(err, syscall.ELOOP),
		strings.Contains(err.Error(), "path escapes"):
		return ErrNotFound
	case errors.Is(err, fs.ErrExist):
		return ErrExists
	default:
		return err
	}
}
