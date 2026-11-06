package events

import "testing"

func TestPublishSubscribe(t *testing.T) {
	h := NewHub(2)
	a, cancelA, ok := h.Subscribe()
	if !ok {
		t.Fatal("subscribe failed")
	}
	b, cancelB, _ := h.Subscribe()
	if _, _, ok := h.Subscribe(); ok {
		t.Error("third subscriber accepted despite max 2")
	}
	h.Publish(Event{Name: "change", Data: 1})
	if e := <-a; e.Name != "change" {
		t.Errorf("a got %+v", e)
	}
	if e := <-b; e.Data != 1 {
		t.Errorf("b got %+v", e)
	}
	cancelA()
	cancelA() // idempotent
	if _, ok := <-a; ok {
		t.Error("channel still open after cancel")
	}
	if h.Subscribers() != 1 {
		t.Errorf("subscribers = %d", h.Subscribers())
	}
	cancelB()
}

func TestSlowSubscriberIsDropped(t *testing.T) {
	h := NewHub(1)
	c, cancel, _ := h.Subscribe()
	defer cancel()
	for i := 0; i <= bufferSize; i++ {
		h.Publish(Event{Name: "x"})
	}
	n := 0
	for range c {
		n++
	}
	if n != bufferSize {
		t.Errorf("received %d events before disconnect, want %d", n, bufferSize)
	}
}

func TestClose(t *testing.T) {
	h := NewHub(4)
	c, cancel, _ := h.Subscribe()
	h.Close()
	if _, ok := <-c; ok {
		t.Error("subscriber not closed")
	}
	cancel() // after Close: no panic
	if _, _, ok := h.Subscribe(); ok {
		t.Error("subscribe after Close succeeded")
	}
	h.Publish(Event{Name: "late"}) // no panic
}
