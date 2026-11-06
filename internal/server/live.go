package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dendronotes/internal/events"
	"dendronotes/internal/httpx"
	"dendronotes/internal/index"
	"dendronotes/internal/watch"
)

const (
	// maxStreams caps simultaneous event streams (browser tabs).
	maxStreams = 64
	// heartbeat keeps proxies from closing idle streams and re-checks the session.
	heartbeat = 25 * time.Second
	// watchDebounce collects bursts of file system events into one batch.
	watchDebounce = 150 * time.Millisecond
)

// runIndex brings the index up to date with the soma, then follows changes
// via the file system watcher and periodic rescans until ctx is done.
func (s *Server) runIndex(ctx context.Context) {
	for attempt := 0; ; attempt++ {
		start := time.Now()
		_, err := s.index.Sync(ctx)
		if err == nil {
			s.log.Info("index synchronized", "took", time.Since(start).Round(time.Millisecond).String())
			break
		}
		if ctx.Err() != nil {
			return
		}
		wait := min(time.Duration(attempt+1)*2*time.Second, 30*time.Second)
		s.log.Error("index sync failed, retrying", "err", err, "in", wait.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}

	w, err := watch.New(watch.Options{
		Dir:      s.soma.Dir(),
		Dirs:     s.somaDirs,
		Skip:     s.hidden,
		Debounce: watchDebounce,
		Log:      s.log,
	})
	if err != nil {
		s.log.Warn("file system watcher unavailable, relying on periodic rescans", "err", err)
	} else {
		runDone := make(chan struct{})
		defer w.Close()
		defer func() { <-runDone }() // no batch may touch the index after we return
		go func() {
			defer close(runDone)
			w.Run(ctx, func(b watch.Batch) { s.applyBatch(ctx, w, b) })
		}()
	}

	if s.cfg.RescanInterval <= 0 {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(s.cfg.RescanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ch, err := s.index.Sync(ctx)
			if err != nil {
				if ctx.Err() == nil {
					s.log.Error("periodic rescan failed", "err", err)
				}
				continue
			}
			if !ch.Empty() {
				s.log.Info("rescan found changes the watcher missed", "notes", len(ch.Notes), "removed", len(ch.Removed))
			}
			s.afterChange(w, ch)
		}
	}
}

// applyBatch re-checks the paths a watcher batch reported.
func (s *Server) applyBatch(ctx context.Context, w *watch.Watcher, b watch.Batch) {
	var (
		ch  index.Changes
		err error
	)
	if b.Rescan {
		ch, err = s.index.Sync(ctx)
	} else {
		ch, err = s.index.Refresh(ctx, b.Paths)
	}
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("index update failed", "err", err)
		}
		return
	}
	s.afterChange(w, ch)
}

func (s *Server) afterChange(w *watch.Watcher, ch index.Changes) {
	if ch.Tree && w != nil {
		w.UpdateDirs()
	}
	s.publish(ch)
}

// somaDirs lists the visible folders of the soma for the watcher.
func (s *Server) somaDirs() ([]string, error) {
	entries, err := s.soma.Tree()
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
func (s *Server) hidden(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if s.soma.Ignored(seg) {
			return true
		}
	}
	return false
}

// publish tells connected clients what changed.
func (s *Server) publish(ch index.Changes) {
	if ch.Empty() {
		return
	}
	if ch.Notes == nil {
		ch.Notes = []index.NoteChange{}
	}
	if ch.Removed == nil {
		ch.Removed = []string{}
	}
	s.hub.Publish(events.Event{Name: "change", Data: ch})
}

// syncAfterWrite updates the index after a request changed the soma's
// structure and informs clients. It runs even if the client has gone away.
func (s *Server) syncAfterWrite(r *http.Request) {
	ch, err := s.index.Sync(context.WithoutCancel(r.Context()))
	if err != nil {
		s.log.Error("index sync after change failed", "err", err)
		return
	}
	ch.Tree = true // folders and attachments are not part of the note diff
	s.publish(ch)
}

// handleEvents streams changes as Server-Sent Events. Clients resynchronize
// (tree, open note) after every "hello", which also follows a reconnect.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ch, cancel, ok := s.hub.Subscribe()
	if !ok {
		httpx.Error(w, http.StatusServiceUnavailable, "too_many_streams")
		return
	}
	defer cancel()

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // nginx ingress: do not buffer the stream
	w.WriteHeader(http.StatusOK)

	send := func(e events.Event) bool {
		data, err := json.Marshal(e.Data)
		if err != nil {
			s.log.Error("event not serializable", "event", e.Name, "err", err)
			return true
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Name, data); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if _, err := fmt.Fprint(w, "retry: 3000\n"); err != nil {
		return
	}
	if !send(events.Event{Name: "hello", Data: map[string]bool{"ready": s.index.Ready()}}) {
		return
	}

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok || !send(e) {
				return // shutdown, too slow, or the client left
			}
		case <-ticker.C:
			if !s.auth.Authenticated(r) {
				return // signed out or session expired
			}
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}
