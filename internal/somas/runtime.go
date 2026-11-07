package somas

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dendronotes/internal/collab"
	"dendronotes/internal/events"
	"dendronotes/internal/index"
	"dendronotes/internal/soma"
	"dendronotes/internal/watch"
)

const (
	// maxStreams caps simultaneous event streams per soma (browser tabs).
	maxStreams = 64
	// watchDebounce collects bursts of file system events into one batch.
	watchDebounce = 150 * time.Millisecond
)

// Runtime is an open soma: its files, search index, live events and the
// notes being edited together.
type Runtime struct {
	ID     string
	Soma   *soma.Soma
	Index  *index.Index
	Hub    *events.Hub
	Collab *collab.Manager
	// MoveMu serializes moves that rewrite synapses in other notes.
	MoveMu sync.Mutex

	log      *slog.Logger
	stop     context.CancelFunc
	done     chan struct{}
	users    atomic.Int64 // requests and streams currently using the soma
	lastUsed atomic.Int64 // unix nanoseconds
}

func newRuntime(id string, sm *soma.Soma, indexDir string, rescan time.Duration, maxNote int64, log *slog.Logger) (*Runtime, error) {
	ix, err := index.Open(indexDir, sm, log)
	if err != nil {
		return nil, err
	}
	ctx, stop := context.WithCancel(context.Background())
	rt := &Runtime{ID: id, Soma: sm, Index: ix, Hub: events.NewHub(maxStreams), log: log, stop: stop, done: make(chan struct{})}
	rt.Collab = collab.New(collab.Options{
		Soma:     sm,
		MaxBytes: maxNote,
		Send:     rt.Hub.Send,
		Saved: func(n soma.Note) {
			ch, err := rt.Index.Put(context.Background(), n)
			if err != nil {
				log.Error("index update after save failed", "path", n.Path, "err", err)
				return
			}
			rt.Publish(ch)
		},
		Log: log,
	})
	rt.touch()
	go func() {
		defer close(rt.done)
		rt.follow(ctx, rescan)
	}()
	return rt, nil
}

// Release ends a use started by Manager.Acquire.
func (rt *Runtime) Release() {
	rt.touch()
	rt.users.Add(-1)
}

func (rt *Runtime) touch() { rt.lastUsed.Store(time.Now().UnixNano()) }

func (rt *Runtime) idleSince(before time.Time) bool {
	return rt.users.Load() == 0 && rt.Hub.Subscribers() == 0 && rt.lastUsed.Load() < before.UnixNano()
}

// DropUser ends the event streams of an account (its access ended); its
// open notes are left.
func (rt *Runtime) DropUser(userID int64) {
	for _, id := range rt.Collab.StreamsOf(userID) {
		rt.Hub.Drop(id)
	}
}

// Streams counts the connected event streams.
func (rt *Runtime) Streams() int { return rt.Hub.Subscribers() }

func (rt *Runtime) close() error {
	rt.Collab.Close() // writes what is not saved yet
	rt.Hub.Close()
	rt.stop()
	<-rt.done
	return errors.Join(rt.Index.Close(), rt.Soma.Close())
}

// follow brings the index up to date with the soma, then follows changes via
// the file system watcher and periodic rescans until ctx is done.
func (rt *Runtime) follow(ctx context.Context, rescan time.Duration) {
	for attempt := 0; ; attempt++ {
		start := time.Now()
		_, err := rt.Index.Sync(ctx)
		if err == nil {
			rt.log.Debug("index synchronized", "took", time.Since(start).Round(time.Millisecond).String())
			break
		}
		if ctx.Err() != nil {
			return
		}
		wait := min(time.Duration(attempt+1)*2*time.Second, 30*time.Second)
		rt.log.Error("index sync failed, retrying", "err", err, "in", wait.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}

	w, err := watch.New(watch.Options{
		Dir:      rt.Soma.Dir(),
		Dirs:     rt.dirs,
		Skip:     rt.hidden,
		Debounce: watchDebounce,
		Log:      rt.log,
	})
	if err != nil {
		rt.log.Warn("file system watcher unavailable, relying on periodic rescans", "err", err)
	} else {
		runDone := make(chan struct{})
		defer w.Close()
		defer func() { <-runDone }() // no batch may touch the index after we return
		go func() {
			defer close(runDone)
			w.Run(ctx, func(b watch.Batch) { rt.applyBatch(ctx, w, b) })
		}()
	}

	if rescan <= 0 {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(rescan)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ch, err := rt.Index.Sync(ctx)
			if err != nil {
				if ctx.Err() == nil {
					rt.log.Error("periodic rescan failed", "err", err)
				}
				continue
			}
			if !ch.Empty() {
				rt.log.Info("rescan found changes the watcher missed", "notes", len(ch.Notes), "removed", len(ch.Removed))
			}
			rt.afterChange(w, ch)
		}
	}
}

func (rt *Runtime) applyBatch(ctx context.Context, w *watch.Watcher, b watch.Batch) {
	var (
		ch  index.Changes
		err error
	)
	if b.Rescan {
		ch, err = rt.Index.Sync(ctx)
	} else {
		ch, err = rt.Index.Refresh(ctx, b.Paths)
	}
	if err != nil {
		if ctx.Err() == nil {
			rt.log.Error("index update failed", "err", err)
		}
		return
	}
	rt.afterChange(w, ch)
}

func (rt *Runtime) afterChange(w *watch.Watcher, ch index.Changes) {
	if ch.Tree && w != nil {
		w.UpdateDirs()
	}
	rt.Publish(ch)
}

// dirs lists the visible folders of the soma for the watcher.
func (rt *Runtime) dirs() ([]string, error) {
	entries, err := rt.Soma.Tree()
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.Type == "dir" {
			dirs = append(dirs, e.Path)
		}
	}
	return dirs, nil
}

// hidden reports whether any segment of a soma-relative path is hidden or ignored.
func (rt *Runtime) hidden(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if rt.Soma.Ignored(seg) {
			return true
		}
	}
	return false
}

// Publish tells connected clients what changed, and lets open documents
// pick up changes made to their files elsewhere.
func (rt *Runtime) Publish(ch index.Changes) {
	if ch.Empty() {
		return
	}
	for _, n := range ch.Notes {
		rt.Collab.FileChanged(n.Path)
	}
	for _, p := range ch.Removed {
		rt.Collab.FileChanged(p)
	}
	if ch.Notes == nil {
		ch.Notes = []index.NoteChange{}
	}
	if ch.Removed == nil {
		ch.Removed = []string{}
	}
	rt.Hub.Publish(events.Event{Name: "change", Data: ch})
}

// SyncAfterWrite updates the index after a request changed the soma's
// structure and informs clients. It runs even if the client has gone away.
func (rt *Runtime) SyncAfterWrite(ctx context.Context) {
	ch, err := rt.Index.Sync(context.WithoutCancel(ctx))
	if err != nil {
		rt.log.Error("index sync after change failed", "err", err)
		return
	}
	ch.Tree = true // folders and attachments are not part of the note diff
	rt.Publish(ch)
}
