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
	"dendronotes/internal/history"
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
	// History keeps earlier versions of the notes (not a cache).
	History *history.Store
	// MoveMu serializes moves that rewrite synapses in other notes.
	MoveMu sync.Mutex

	pruned atomic.Int64 // when the history was last pruned (unix nanoseconds)

	log      *slog.Logger
	stop     context.CancelFunc
	done     chan struct{}
	users    atomic.Int64 // requests and streams currently using the soma
	lastUsed atomic.Int64 // unix nanoseconds
}

func newRuntime(id string, sm *soma.Soma, indexDir, historyFile string, rescan time.Duration, maxNote int64, log *slog.Logger) (*Runtime, error) {
	hist, err := history.Open(historyFile)
	if err != nil {
		return nil, err
	}
	ix, err := index.Open(indexDir, sm, log)
	if err != nil {
		hist.Close()
		return nil, err
	}
	ctx, stop := context.WithCancel(context.Background())
	rt := &Runtime{ID: id, Soma: sm, Index: ix, History: hist, Hub: events.NewHub(maxStreams), log: log, stop: stop, done: make(chan struct{})}
	rt.Collab = collab.New(collab.Options{
		Soma:     sm,
		MaxBytes: maxNote,
		Send:     rt.Hub.Send,
		Saved: func(n soma.Note, content []byte, authors []int64) {
			rt.RecordVersion(n.Path, content, authors, history.ReasonEdit)
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
	return errors.Join(rt.Index.Close(), rt.History.Close(), rt.Soma.Close())
}

// RecordVersion adds content to the history of a note (errors are logged:
// a missing version must not fail a save).
func (rt *Runtime) RecordVersion(p string, content []byte, authors []int64, reason string) {
	if err := rt.History.Record(context.Background(), p, content, authors, reason); err != nil {
		rt.log.Error("recording a version failed", "path", p, "err", err)
	}
}

// MoveHistory lets the history of from (a note or folder) follow it to to.
func (rt *Runtime) MoveHistory(from, to string) {
	if err := rt.History.Move(context.Background(), from, to); err != nil {
		rt.log.Error("moving history failed", "from", from, "to", to, "err", err)
	}
}

// RemoveHistory forgets the history of p (a note or folder).
func (rt *Runtime) RemoveHistory(p string) {
	if err := rt.History.Remove(context.Background(), p); err != nil {
		rt.log.Error("removing history failed", "path", p, "err", err)
	}
}

// pruneHistory removes versions older than days (once a day at most).
func (rt *Runtime) pruneHistory(days int) {
	if days <= 0 || time.Since(time.Unix(0, rt.pruned.Load())) < 24*time.Hour {
		return
	}
	rt.pruned.Store(time.Now().UnixNano())
	n, err := rt.History.Prune(context.Background(), time.Now().AddDate(0, 0, -days))
	switch {
	case err != nil:
		rt.log.Error("pruning history failed", "err", err)
	case n > 0:
		rt.log.Info("history pruned", "versions", n)
	}
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
	// Notes changed outside dendronotes (own writes are recorded when made,
	// so these find their content already there and are skipped).
	for _, n := range ch.Notes {
		if note, err := rt.Soma.ReadNote(n.Path); err == nil {
			rt.RecordVersion(n.Path, []byte(note.Content), nil, history.ReasonOutside)
		}
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
