// Package watch reports changes in the soma made outside dendronotes (a text
// editor, git, Syncthing …) via fsnotify (inotify on Linux).
//
// Events are only hints: the receiver re-checks the reported paths through
// the soma. When events may have been lost (queue overflow), the batch asks
// for a complete rescan; a periodic rescan in the server covers file systems
// that deliver no events at all (e.g. some network volumes).
package watch

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Options configure a watcher.
type Options struct {
	// Dir is the absolute path of the soma.
	Dir string
	// Dirs lists the soma-relative folders to watch (without the root).
	Dirs func() ([]string, error)
	// Skip reports whether a soma-relative path is hidden or ignored.
	Skip func(rel string) bool
	// Debounce is the quiet time before a batch is delivered.
	Debounce time.Duration
	Log      *slog.Logger
}

// Batch is a set of changed paths.
type Batch struct {
	Paths  []string // soma-relative, sorted, unique
	Rescan bool     // events were lost: compare everything
}

// Watcher watches every visible folder of the soma.
type Watcher struct {
	opts Options
	fsw  *fsnotify.Watcher

	mu      sync.Mutex
	watched map[string]bool // absolute folder paths
}

// maxDelay bounds how long a busy stream of events can postpone a batch.
const maxDelay = time.Second

// New starts watching the soma root and all its visible folders.
func New(opts Options) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{opts: opts, fsw: fsw, watched: map[string]bool{}}
	if err := w.add(opts.Dir); err != nil {
		fsw.Close()
		return nil, err
	}
	w.UpdateDirs()
	return w, nil
}

func (w *Watcher) add(abs string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.watched[abs] {
		return nil
	}
	if err := w.fsw.Add(abs); err != nil {
		return err
	}
	w.watched[abs] = true
	return nil
}

// UpdateDirs watches folders that appeared since the last call.
func (w *Watcher) UpdateDirs() {
	dirs, err := w.opts.Dirs()
	if err != nil {
		w.opts.Log.Warn("watch: cannot list folders", "err", err)
		return
	}
	failed := 0
	for _, d := range dirs {
		if err := w.add(filepath.Join(w.opts.Dir, filepath.FromSlash(d))); err != nil {
			failed++
			if failed == 1 {
				// Typically the inotify watch limit (fs.inotify.max_user_watches).
				w.opts.Log.Warn("watch: cannot watch folder, relying on periodic rescans", "dir", d, "err", err)
			}
		}
	}
}

// Close stops watching.
func (w *Watcher) Close() error { return w.fsw.Close() }

// Run delivers batches to out until ctx is done or the watcher is closed.
func (w *Watcher) Run(ctx context.Context, out func(Batch)) {
	pending := map[string]bool{}
	rescan := false
	var first time.Time
	timer := time.NewTimer(time.Hour)
	timer.Stop()

	flush := func() {
		if len(pending) == 0 && !rescan {
			return
		}
		b := Batch{Rescan: rescan}
		for p := range pending {
			b.Paths = append(b.Paths, p)
		}
		slices.Sort(b.Paths)
		clear(pending)
		rescan = false
		first = time.Time{}
		out(b)
	}
	schedule := func() {
		now := time.Now()
		if first.IsZero() {
			first = now
		}
		wait := w.opts.Debounce
		if remaining := maxDelay - now.Sub(first); remaining < wait {
			wait = max(remaining, 0)
		}
		timer.Reset(wait)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			flush()
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if ev.Has(fsnotify.Chmod) && !ev.Has(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) {
				continue
			}
			rel, ok := w.rel(ev.Name)
			if !ok {
				continue
			}
			if rel == "" {
				rescan = true // the root itself
			} else {
				if w.opts.Skip(rel) {
					continue
				}
				pending[rel] = true
			}
			if ev.Has(fsnotify.Remove | fsnotify.Rename) {
				w.forget(ev.Name)
			}
			schedule()
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				w.opts.Log.Warn("watch: event queue overflowed, rescanning")
			} else {
				w.opts.Log.Warn("watch: error", "err", err)
			}
			rescan = true
			schedule()
		}
	}
}

// rel turns an absolute event path into a soma-relative one.
func (w *Watcher) rel(abs string) (string, bool) {
	r, err := filepath.Rel(w.opts.Dir, abs)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	if r == "." {
		return "", true
	}
	return filepath.ToSlash(r), true
}

// forget drops a removed folder (and everything below it) from the watch list.
func (w *Watcher) forget(abs string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	prefix := abs + string(filepath.Separator)
	for d := range w.watched {
		if d == abs || strings.HasPrefix(d, prefix) {
			delete(w.watched, d)
			_ = w.fsw.Remove(d)
		}
	}
}
