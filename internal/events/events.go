// Package events fans out live updates to connected clients (Server-Sent Events).
package events

import "sync"

// Event is one message to clients.
type Event struct {
	Name string // SSE event name
	Data any    // marshalled as JSON
}

// Hub distributes events to subscribers. A subscriber that cannot keep up is
// disconnected; its client reconnects and resynchronizes.
type Hub struct {
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	max    int
	closed bool
}

// bufferSize is how many events a subscriber may fall behind.
const bufferSize = 32

// NewHub allows at most max simultaneous subscribers.
func NewHub(max int) *Hub {
	return &Hub{subs: map[chan Event]struct{}{}, max: max}
}

// Subscribe registers a subscriber. ok is false when the hub is closed or full.
// cancel must be called when the subscriber leaves.
func (h *Hub) Subscribe() (ch <-chan Event, cancel func(), ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.subs) >= h.max {
		return nil, func() {}, false
	}
	c := make(chan Event, bufferSize)
	h.subs[c] = struct{}{}
	return c, func() { h.drop(c) }, true
}

func (h *Hub) drop(c chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[c]; ok {
		delete(h.subs, c)
		close(c)
	}
}

// Publish sends e to every subscriber without blocking.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.subs {
		select {
		case c <- e:
		default: // too slow: disconnect
			delete(h.subs, c)
			close(c)
		}
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
	for c := range h.subs {
		delete(h.subs, c)
		close(c)
	}
}
