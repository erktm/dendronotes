package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/collab"
	"dendronotes/internal/events"
	"dendronotes/internal/httpx"
)

// Long polling is the fallback for networks that hold back Server-Sent
// Events: proxies or security software that pass on a response only once it
// is complete, which a live stream never is. A polled stream is a hub
// subscriber like an event stream and gets the same events; between two polls
// they wait in its queue. Every poll is an ordinary request that ends.
//
// A polled stream outlives the requests that read it, so it keeps its hub
// subscription and collab stream between them. The soma stays open meanwhile
// (a subscriber keeps it from going idle); when it closes anyway (deleted,
// shutdown), the hub drops the subscriber and the next poll answers 410.

var (
	// pollWait is how long a poll waits for events before it answers empty.
	pollWait = 25 * time.Second
	// pollExpiry ends a stream that nobody polled for this long (the tab is gone).
	pollExpiry = 60 * time.Second
)

// pollMaxQueue bounds the events waiting for a client; beyond it the stream
// ends and the client starts a new one (and resynchronizes).
const pollMaxQueue = 2048

type polledEvent struct {
	Seq  int64           `json:"seq"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type pollResponse struct {
	Stream string        `json:"stream"`
	Events []polledEvent `json:"events"`
}

type pollStream struct {
	id     string
	somaID string
	userID int64
	expiry *time.Timer
	end    func() // ends the subscription and the collab stream; idempotent

	mu    sync.Mutex
	queue []polledEvent
	seq   int64         // number of the last event queued
	gone  bool          // the hub dropped the stream, or the queue overflowed
	wake  chan struct{} // closed (and replaced) when an event arrives
}

// openPoll subscribes a new polled stream to the soma's events.
func (s *Server) openPoll(r *http.Request) (*pollStream, bool) {
	rt := runtimeOf(r)
	sm, _ := somaOf(r)
	u, _ := auth.UserFrom(r.Context())
	id, ch, cancel, ok := rt.Hub.Subscribe()
	if !ok {
		return nil, false
	}
	cl := rt.Collab
	cl.AddStream(id, collab.User{ID: u.ID, Name: u.Name(), Avatar: u.Avatar})

	ps := &pollStream{id: id, somaID: sm.ID, userID: u.ID, wake: make(chan struct{})}
	var once sync.Once
	ps.end = func() {
		once.Do(func() {
			cancel()
			cl.RemoveStream(id)
			s.pollMu.Lock()
			delete(s.polls, id)
			s.pollMu.Unlock()
		})
	}
	ps.expiry = time.AfterFunc(pollExpiry, ps.end)
	s.pollMu.Lock()
	if s.polls == nil {
		s.polls = map[string]*pollStream{}
	}
	s.polls[id] = ps
	s.pollMu.Unlock()
	go ps.drain(ch)
	return ps, true
}

// drain moves the hub's events into the queue, so that the hub never finds
// the stream too slow while its client is between two polls.
func (ps *pollStream) drain(ch <-chan events.Event) {
	for e := range ch {
		data, err := json.Marshal(e.Data)
		if err != nil {
			continue
		}
		ps.mu.Lock()
		if len(ps.queue) >= pollMaxQueue {
			ps.gone = true
			ps.notifyLocked()
			ps.mu.Unlock()
			ps.end()
			return
		}
		ps.seq++
		ps.queue = append(ps.queue, polledEvent{Seq: ps.seq, Name: e.Name, Data: data})
		ps.notifyLocked()
		ps.mu.Unlock()
	}
	ps.mu.Lock()
	ps.gone = true
	ps.notifyLocked()
	ps.mu.Unlock()
}

func (ps *pollStream) notifyLocked() {
	close(ps.wake)
	ps.wake = make(chan struct{})
}

// take drops the events the client has (up to after) and returns the rest.
func (ps *pollStream) take(after int64) (evs []polledEvent, gone bool, wake <-chan struct{}) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	n := 0
	for n < len(ps.queue) && ps.queue[n].Seq <= after {
		n++
	}
	ps.queue = ps.queue[n:]
	return append([]polledEvent(nil), ps.queue...), ps.gone, ps.wake
}

// handlePoll is the long-polling form of handleEvents. Without a stream it
// opens one and answers with its "hello"; with one it answers the events
// after the given number, waiting up to pollWait for some.
func (s *Server) handlePoll(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("stream")
	if id == "" {
		ps, ok := s.openPoll(r)
		if !ok {
			httpx.Error(w, http.StatusServiceUnavailable, "too_many_streams")
			return
		}
		hello, _ := json.Marshal(map[string]any{"ready": runtimeOf(r).Index.Ready(), "stream": ps.id})
		httpx.WriteJSON(w, http.StatusOK, pollResponse{Stream: ps.id, Events: []polledEvent{{Name: "hello", Data: hello}}})
		return
	}

	sm, _ := somaOf(r)
	u, _ := auth.UserFrom(r.Context())
	s.pollMu.Lock()
	ps := s.polls[id]
	s.pollMu.Unlock()
	// Only its own account reads a stream; to anyone else it does not exist.
	if ps == nil || ps.somaID != sm.ID || ps.userID != u.ID {
		httpx.Error(w, http.StatusGone, "stream_gone")
		return
	}
	after, _ := strconv.ParseInt(q.Get("after"), 10, 64)
	ps.expiry.Reset(pollExpiry)
	defer ps.expiry.Reset(pollExpiry)

	evs, gone, wake := ps.take(after)
	if len(evs) == 0 && !gone {
		timer := time.NewTimer(pollWait)
		select {
		case <-wake:
		case <-timer.C:
		case <-r.Context().Done():
		}
		timer.Stop()
		evs, gone, _ = ps.take(after)
	}
	if len(evs) == 0 && gone {
		ps.end()
		httpx.Error(w, http.StatusGone, "stream_gone")
		return
	}
	if evs == nil {
		evs = []polledEvent{}
	}
	httpx.WriteJSON(w, http.StatusOK, pollResponse{Stream: ps.id, Events: evs})
}
