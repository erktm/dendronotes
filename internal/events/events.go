// Package events fans out live updates to connected clients (Server-Sent Events).
package events

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
)

// Event is one message to clients.
type Event struct {
	Name string // SSE event name
	Data any    // marshalled as JSON
}

// Hub distributes events to subscribers. A subscriber that cannot keep up is
// disconnected; its client reconnects and resynchronizes.
type Hub struct {
	mu     sync.Mutex
	subs   map[string]chan Event
	max    int
	closed bool
}

// bufferSize is how many events a subscriber may fall behind. Collaborative
// editing sends an event per typing burst, so there is room for a few seconds.
const bufferSize = 256

// NewHub allows at most max simultaneous subscribers.
func NewHub(max int) *Hub {
	return &Hub{subs: map[string]chan Event{}, max: max}
}

// Subscribe registers a subscriber under a new random id. ok is false when
// the hub is closed or full. cancel must be called when the subscriber leaves.
func (h *Hub) Subscribe() (id string, ch <-chan Event, cancel func(), ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.subs) >= h.max {
		return "", nil, func() {}, false
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	id = base64.RawURLEncoding.EncodeToString(b[:])
	c := make(chan Event, bufferSize)
	h.subs[id] = c
	return id, c, func() { h.Drop(id) }, true
}

// Drop disconnects a subscriber (idempotent).
func (h *Hub) Drop(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.subs[id]; ok {
		delete(h.subs, id)
		close(c)
	}
}

// Publish sends e to every subscriber without blocking.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, c := range h.subs {
		h.deliver(id, c, e)
	}
}

// Send delivers e to some subscribers only, without blocking.
func (h *Hub) Send(ids []string, e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, id := range ids {
		if c, ok := h.subs[id]; ok {
			h.deliver(id, c, e)
		}
	}
}

func (h *Hub) deliver(id string, c chan Event, e Event) {
	select {
	case c <- e:
	default: // too slow: disconnect
		delete(h.subs, id)
		close(c)
	}
}

// Subscribers returns the number of connected subscribers.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Close disconnects all subscribers and refuses new ones (on shutdown).
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for id, c := range h.subs {
		delete(h.subs, id)
		close(c)
	}
}
