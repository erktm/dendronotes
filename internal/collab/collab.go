// Package collab lets several people edit the same note at once. Every note
// someone has open is a live document on the server: a version number and
// the list of updates (CodeMirror change sets) that led there. A client
// pushes its updates for the version it has seen; if others were faster, it
// is refused, catches up and rebases (the model of @codemirror/collab). The
// server only checks and applies updates, sends them to everyone in the
// note, and writes the document back to its Markdown file shortly after
// changes stop. Changes made to the file elsewhere are merged in.
package collab

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"dendronotes/internal/events"
	"dendronotes/internal/soma"
	"dendronotes/internal/store"
)

// Errors for the HTTP layer (besides those of internal/soma).
var (
	ErrUnknownStream = errors.New("unknown event stream")
	ErrResync        = errors.New("document changed underneath; join again")
	ErrVersion       = errors.New("not based on the current version")
	ErrInvalid       = errors.New("invalid update")
	ErrRateLimited   = errors.New("too many updates")
	ErrClosed        = errors.New("closed")
)

const (
	// keepUpdates and keepBytes bound the history kept for clients that
	// catch up (pull); older clients join again.
	keepUpdates = 500
	keepBytes   = 8 << 20
	// maxPeerName bounds names shown next to cursors.
	maxPeerName = 80
	colors      = 8
)

// User is the account behind an event stream.
type User struct {
	ID     int64
	Name   string
	Avatar store.Avatar
}

// Update is one change set of one client, in CodeMirror's JSON form.
type Update struct {
	ClientID string          `json:"clientID"`
	Changes  json.RawMessage `json:"changes"`
}

// Selection is a cursor (anchor == head) or a selected range.
type Selection struct {
	Anchor int `json:"anchor"`
	Head   int `json:"head"`
}

// Peer is someone in a note, as others see them.
type Peer struct {
	ID   string `json:"id"`
	User int64  `json:"user"`
	Name string `json:"name"`
	// Avatar of the account, shown with the others in the note.
	Avatar store.Avatar `json:"avatar"`
	Color  int          `json:"color"`
	Sel    *Selection   `json:"sel,omitempty"`
}

// Recover carries the state of a client that lost its document (server
// restart, or offline too long): the text it last had confirmed and its
// own text. Its changes are merged into the current document.
type Recover struct {
	Doc  string `json:"doc"`
	Base string `json:"base"`
	Mine string `json:"mine"`
}

// Joined is what a client needs to start editing.
type Joined struct {
	Doc      string `json:"doc"`
	Version  int    `json:"version"`
	Text     string `json:"text"`
	ReadOnly bool   `json:"readOnly"`
	Saved    int    `json:"saved"` // version written to disk
	Peer     string `json:"peer"`  // the caller's own peer id
	Peers    []Peer `json:"peers"`
	Conflict bool   `json:"conflict,omitempty"` // recovered changes clashed and were kept side by side
}

// Options configure a manager (one per open soma).
type Options struct {
	Soma     *soma.Soma
	MaxBytes int64
	// Send delivers an event to the given streams (non-blocking).
	Send func(streams []string, e events.Event)
	// Saved runs after the manager wrote a note (index update, change events).
	Saved     func(n soma.Note)
	Log       *slog.Logger
	SaveDelay time.Duration // after the last change (default 1s)
	MaxDelay  time.Duration // after the first unsaved change (default 5s)
	// Linger keeps a document open after the last tab left (default 60s),
	// so that a tab reconnecting after a short break simply continues.
	Linger time.Duration
}

// Manager holds the live documents of one soma.
type Manager struct {
	opts Options

	mu      sync.Mutex
	docs    map[string]*doc
	streams map[string]*stream
	closed  bool

	// Disk checks run in a worker, so that callers (index updates, which
	// may run while a document is being saved) never wait for locks.
	cmu     sync.Mutex
	pending map[string]bool
	wake    chan struct{}
	done    chan struct{}
}

type stream struct {
	id      string
	user    User
	docs    map[string]bool
	updates *rate.Limiter
	cursors *rate.Limiter
}

// New creates a manager.
func New(opts Options) *Manager {
	if opts.SaveDelay <= 0 {
		opts.SaveDelay = time.Second
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = 5 * time.Second
	}
	if opts.Linger <= 0 {
		opts.Linger = time.Minute
	}
	m := &Manager{
		opts: opts, docs: map[string]*doc{}, streams: map[string]*stream{},
		pending: map[string]bool{}, wake: make(chan struct{}, 1), done: make(chan struct{}),
	}
	go m.checker(m.wake)
	return m
}

// AddStream registers an event stream (a browser tab) of an account.
func (m *Manager) AddStream(id string, u User) {
	if len(u.Name) > maxPeerName {
		u.Name = u.Name[:maxPeerName]
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streams[id] = &stream{
		id: id, user: u, docs: map[string]bool{},
		updates: rate.NewLimiter(50, 200),
		cursors: rate.NewLimiter(20, 40),
	}
}

// RemoveStream forgets a stream and leaves every note it had joined.
func (m *Manager) RemoveStream(id string) {
	m.mu.Lock()
	st := m.streams[id]
	delete(m.streams, id)
	var docs []*doc
	if st != nil {
		for p := range st.docs {
			if d := m.docs[p]; d != nil {
				docs = append(docs, d)
			}
		}
	}
	m.mu.Unlock()
	for _, d := range docs {
		m.leave(d, id)
	}
}

// StreamsOf lists the streams of an account (to end them when its access ends).
func (m *Manager) StreamsOf(userID int64) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for id, st := range m.streams {
		if st.user.ID == userID {
			out = append(out, id)
		}
	}
	return out
}

// streamOf checks that a stream exists and belongs to the account.
func (m *Manager) streamOf(id string, userID int64) (*stream, error) {
	st := m.streams[id]
	if st == nil || st.user.ID != userID {
		return nil, ErrUnknownStream
	}
	return st, nil
}

// Join opens a note for a stream: the document as it is now, live updates
// from here on. With rec, changes the client made against a lost document
// are merged in first.
func (m *Manager) Join(streamID string, userID int64, path string, rec *Recover) (Joined, error) {
	path, err := soma.CleanPath(path)
	if err != nil {
		return Joined{}, err
	}
	if !soma.IsNote(path) {
		return Joined{}, soma.ErrNotNote
	}
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return Joined{}, ErrClosed
		}
		st, err := m.streamOf(streamID, userID)
		if err != nil {
			m.mu.Unlock()
			return Joined{}, err
		}
		d := m.docs[path]
		if d == nil {
			d = &doc{m: m, path: path, peers: map[string]*peer{}}
			m.docs[path] = d
		}
		st.docs[path] = true
		m.mu.Unlock()

		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			continue // raced with the last one leaving: start over
		}
		if !d.loaded {
			if err := d.load(); err != nil {
				d.closed = true
				d.mu.Unlock()
				m.forget(d)
				m.mu.Lock()
				delete(st.docs, path)
				m.mu.Unlock()
				return Joined{}, err
			}
		}
		res := d.join(st, rec)
		d.mu.Unlock()
		return res, nil
	}
}

// Leave closes a note for a stream.
func (m *Manager) Leave(streamID string, userID int64, path string) error {
	m.mu.Lock()
	st, err := m.streamOf(streamID, userID)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	d := m.docs[path]
	delete(st.docs, path)
	m.mu.Unlock()
	if d != nil {
		m.leave(d, streamID)
	}
	return nil
}

// leave removes a peer. When the last one is gone, the document is saved
// and closed after Linger, unless someone joins again before.
func (m *Manager) leave(d *doc, streamID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.peers[streamID]; !ok || d.closed {
		return
	}
	delete(d.peers, streamID)
	if len(d.peers) > 0 {
		d.sendPeers()
		return
	}
	d.save()
	if d.idle != nil {
		d.idle.Stop()
	}
	d.idle = time.AfterFunc(m.opts.Linger, func() {
		d.mu.Lock()
		closing := len(d.peers) == 0 && !d.closed
		if closing {
			d.save()
			d.stop()
			d.closed = true
		}
		d.mu.Unlock()
		if closing {
			m.forget(d)
		}
	})
}

func (m *Manager) forget(d *doc) {
	m.mu.Lock()
	if m.docs[d.path] == d {
		delete(m.docs, d.path)
	}
	m.mu.Unlock()
}

// joined returns the document a stream has joined.
func (m *Manager) joined(streamID string, userID int64, path string) (*doc, *stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, err := m.streamOf(streamID, userID)
	if err != nil {
		return nil, nil, err
	}
	d := m.docs[path]
	if d == nil || !st.docs[path] {
		return nil, nil, ErrResync
	}
	return d, st, nil
}

// Push applies a client's updates made on top of version. It returns the
// new version; ErrVersion means others were faster (catch up, rebase, retry).
func (m *Manager) Push(streamID string, userID int64, path, docID string, version int, updates []Update, saveNow bool) (int, error) {
	d, st, err := m.joined(streamID, userID, path)
	if err != nil {
		return 0, err
	}
	if !st.updates.AllowN(time.Now(), max(len(updates), 1)) {
		return 0, ErrRateLimited
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case d.closed || d.id != docID || version > d.version:
		return 0, ErrResync
	case d.readOnly:
		return 0, soma.ErrReadOnly
	case version < d.version:
		return d.version, ErrVersion
	}
	text := d.text
	for _, u := range updates {
		if len(u.ClientID) > 64 {
			return 0, ErrInvalid
		}
		ch, err := parseChanges(u.Changes)
		if err != nil {
			return 0, ErrInvalid
		}
		if text, err = ch.apply(text); err != nil {
			return 0, ErrInvalid
		}
	}
	if int64(len(text)) > m.opts.MaxBytes/3 && utf8Len(text) > m.opts.MaxBytes {
		return 0, soma.ErrTooLarge
	}
	d.commit(text, updates)
	if saveNow {
		d.save()
	}
	return d.version, nil
}

// Pull returns the updates after version (to catch up).
func (m *Manager) Pull(path, docID string, version int) ([]Update, int, error) {
	m.mu.Lock()
	d := m.docs[path]
	m.mu.Unlock()
	if d == nil {
		return nil, 0, ErrResync
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.id != docID || version < d.first || version > d.version {
		return nil, 0, ErrResync
	}
	out := append([]Update{}, d.updates[version-d.first:]...)
	return out, d.version, nil
}

// Presence shares where a peer's cursor is (nil: not in the editor).
func (m *Manager) Presence(streamID string, userID int64, path, docID string, sel *Selection) error {
	d, st, err := m.joined(streamID, userID, path)
	if err != nil {
		return err
	}
	if !st.cursors.Allow() {
		return ErrRateLimited
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.id != docID {
		return ErrResync
	}
	p := d.peers[streamID]
	if p == nil {
		return ErrResync
	}
	if sel != nil {
		n := len(d.text)
		s := Selection{Anchor: min(max(sel.Anchor, 0), n), Head: min(max(sel.Head, 0), n)}
		sel = &s
	}
	p.sel = sel
	d.sendPeers()
	return nil
}

// FileChanged reports that a note may have changed on disk; an open
// document checks the file and merges changes made elsewhere. It returns at
// once and takes no locks besides its own queue.
func (m *Manager) FileChanged(path string) {
	m.cmu.Lock()
	defer m.cmu.Unlock()
	m.pending[path] = true
	if m.wake == nil {
		return // closed
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) checker(wake <-chan struct{}) {
	defer close(m.done)
	for range wake {
		m.cmu.Lock()
		paths := m.pending
		m.pending = map[string]bool{}
		m.cmu.Unlock()
		for p := range paths {
			m.mu.Lock()
			d := m.docs[p]
			m.mu.Unlock()
			if d == nil {
				continue
			}
			d.mu.Lock()
			gone := d.checkDisk()
			d.mu.Unlock()
			if gone {
				m.forget(d)
			}
		}
	}
}

// Flush saves unsaved documents at path or below it now.
func (m *Manager) Flush(path string) {
	for _, d := range m.under(path) {
		d.mu.Lock()
		d.save()
		d.mu.Unlock()
	}
}

func (m *Manager) under(path string) []*doc {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*doc
	for p, d := range m.docs {
		if path == "" || p == path || strings.HasPrefix(p, path+"/") {
			out = append(out, d)
		}
	}
	return out
}

// Move saves the documents at or below from, runs move (which renames the
// file or folder and returns the new path) and carries open documents over
// to their new paths. Checks of the old paths wait until it is done.
func (m *Manager) Move(from string, move func() (string, error)) (string, error) {
	m.Flush(from)
	m.mu.Lock()
	defer m.mu.Unlock()
	to, err := move()
	if err != nil {
		return "", err
	}
	for p, d := range m.docs {
		if p != from && !strings.HasPrefix(p, from+"/") {
			continue
		}
		np := to + p[len(from):]
		delete(m.docs, p)
		m.docs[np] = d
		for _, st := range m.streams {
			if st.docs[p] {
				delete(st.docs, p)
				st.docs[np] = true
			}
		}
		d.mu.Lock()
		d.path = np
		d.notice("renamed", map[string]any{"from": p, "to": np})
		d.mu.Unlock()
	}
	return to, nil
}

// Remove saves the documents at or below path, runs remove (moving them to
// the trash) and closes them.
func (m *Manager) Remove(path string, remove func() error) error {
	m.Flush(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := remove(); err != nil {
		return err
	}
	for p, d := range m.docs {
		if p != path && !strings.HasPrefix(p, path+"/") {
			continue
		}
		delete(m.docs, p)
		for _, st := range m.streams {
			delete(st.docs, p)
		}
		d.mu.Lock()
		d.notice("removed", nil)
		d.stop()
		d.closed = true
		d.mu.Unlock()
	}
	return nil
}

// Close saves every document and stops (the soma closes).
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	docs := make([]*doc, 0, len(m.docs))
	for _, d := range m.docs {
		docs = append(docs, d)
	}
	m.mu.Unlock()
	m.cmu.Lock()
	if m.wake != nil {
		close(m.wake)
		m.wake = nil
	}
	m.cmu.Unlock()
	<-m.done
	for _, d := range docs {
		d.mu.Lock()
		d.save()
		d.stop()
		d.closed = true
		d.mu.Unlock()
	}
}

// Open lists the paths of the open documents and their number of peers.
func (m *Manager) Open() map[string]int {
	m.mu.Lock()
	docs := make([]*doc, 0, len(m.docs))
	for _, d := range m.docs {
		docs = append(docs, d)
	}
	m.mu.Unlock()
	out := map[string]int{}
	for _, d := range docs {
		d.mu.Lock()
		out[d.path] = len(d.peers)
		d.mu.Unlock()
	}
	return out
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
