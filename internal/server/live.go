package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/collab"
	"dendronotes/internal/events"
	"dendronotes/internal/httpx"
)

// heartbeat keeps proxies from closing idle streams and re-checks the
// session and the membership.
const heartbeat = 25 * time.Second

// handleEvents streams changes as Server-Sent Events. Clients resynchronize
// (tree, open note) after every "hello", which also follows a reconnect.
// The "hello" names the stream; a tab uses that id to join notes for
// editing together, whose updates then arrive on this stream as well.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	sm, _ := somaOf(r)
	id, ch, cancel, ok := rt.Hub.Subscribe()
	if !ok {
		httpx.Error(w, http.StatusServiceUnavailable, "too_many_streams")
		return
	}
	defer cancel()
	u, _ := auth.UserFrom(r.Context())
	rt.Collab.AddStream(id, collab.User{ID: u.ID, Name: u.Name(), Avatar: u.Avatar})
	defer rt.Collab.RemoveStream(id)

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
	if !send(events.Event{Name: "hello", Data: map[string]any{"ready": rt.Index.Ready(), "stream": id}}) {
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
			// Sharing changed (also through a group): end the stream at once
			// when this account has lost access.
			if e.Name == "access" && !s.stillMember(r, sm.ID) {
				return
			}
		case <-ticker.C:
			if !s.stillMember(r, sm.ID) {
				return // signed out, session expired, or no longer a member
			}
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

// stillMember re-checks a long-lived request: the session is valid and the
// account still has access to the soma.
func (s *Server) stillMember(r *http.Request, somaID string) bool {
	if !s.auth.Authenticated(r) {
		return false
	}
	u, _ := auth.UserFrom(r.Context())
	role, err := s.store.EffectiveRole(r.Context(), somaID, u.ID)
	return err == nil && role != ""
}
