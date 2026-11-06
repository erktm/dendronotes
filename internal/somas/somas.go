// Package somas manages the somas of all users: each soma is a folder in
// SOMAS_DIR (or an imported folder elsewhere), registered in the store. A
// soma is opened on first use – files, search index, watcher, live events –
// and closed again after a while without use.
package somas

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"dendronotes/internal/soma"
	"dendronotes/internal/store"
)

// Options configure the manager.
type Options struct {
	SomasDir       string
	DataDir        string // search indexes go to DataDir/index/<soma id>/
	Soma           soma.Options
	RescanInterval time.Duration
	// IdleAfter closes a soma nobody used for this long (0: 30 minutes).
	IdleAfter time.Duration
	Log       *slog.Logger
}

// Manager opens and closes somas on demand.
type Manager struct {
	opts  Options
	store *store.Store

	mu     sync.Mutex
	open   map[string]*Runtime
	closed bool

	stop context.CancelFunc
	done chan struct{}
}

// TrashDir is where deleted somas go, inside SOMAS_DIR (hidden).
const TrashDir = ".trash"

// New starts a manager.
func New(st *store.Store, opts Options) *Manager {
	if opts.IdleAfter <= 0 {
		opts.IdleAfter = 30 * time.Minute
	}
	ctx, stop := context.WithCancel(context.Background())
	m := &Manager{opts: opts, store: st, open: map[string]*Runtime{}, stop: stop, done: make(chan struct{})}
	go m.reap(ctx)
	return m
}

// Path returns the folder of a soma.
func (m *Manager) Path(s store.Soma) string {
	if filepath.IsAbs(s.Folder) {
		return s.Folder
	}
	return filepath.Join(m.opts.SomasDir, s.Folder)
}

func (m *Manager) indexDir(id string) string {
	return filepath.Join(m.opts.DataDir, "index", id)
}

// ErrClosed is returned after Close.
var ErrClosed = errors.New("server is shutting down")

// Acquire opens a soma (if needed) and marks it in use; call Release when done.
func (m *Manager) Acquire(s store.Soma) (*Runtime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	rt := m.open[s.ID]
	if rt == nil {
		var err error
		if rt, err = m.start(s); err != nil {
			return nil, err
		}
		m.open[s.ID] = rt
	}
	rt.users.Add(1)
	rt.touch()
	return rt, nil
}

func (m *Manager) start(s store.Soma) (*Runtime, error) {
	dir := m.indexDir(s.ID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	sm, err := soma.Open(m.Path(s), m.opts.Soma)
	if err != nil {
		return nil, fmt.Errorf("open soma %s: %w", s.ID, err)
	}
	rt, err := newRuntime(s.ID, sm, dir, m.opts.RescanInterval, m.opts.Log.With("soma", s.ID))
	if err != nil {
		sm.Close()
		return nil, err
	}
	m.opts.Log.Debug("soma opened", "soma", s.ID, "name", s.Name)
	return rt, nil
}

// Running returns a soma's runtime if it is open (for statistics), without
// opening it or marking it in use.
func (m *Manager) Running() []*Runtime {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Runtime, 0, len(m.open))
	for _, rt := range m.open {
		out = append(out, rt)
	}
	return out
}

// reap closes somas that have been idle for a while.
func (m *Manager) reap(ctx context.Context) {
	defer close(m.done)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.closeIdle(time.Now().Add(-m.opts.IdleAfter))
		}
	}
}

func (m *Manager) closeIdle(before time.Time) {
	m.mu.Lock()
	var idle []*Runtime
	for id, rt := range m.open {
		if rt.idleSince(before) {
			delete(m.open, id)
			idle = append(idle, rt)
		}
	}
	m.mu.Unlock()
	for _, rt := range idle {
		if err := rt.close(); err != nil {
			m.opts.Log.Error("closing soma failed", "soma", rt.ID, "err", err)
		}
	}
}

// close takes a soma out of service (before deleting it).
func (m *Manager) closeOne(id string) error {
	m.mu.Lock()
	rt := m.open[id]
	delete(m.open, id)
	m.mu.Unlock()
	if rt == nil {
		return nil
	}
	return rt.close()
}

// Close shuts every open soma down.
func (m *Manager) Close() error {
	m.stop()
	<-m.done
	m.mu.Lock()
	m.closed = true
	all := m.open
	m.open = map[string]*Runtime{}
	m.mu.Unlock()
	var errs []error
	for _, rt := range all {
		errs = append(errs, rt.close())
	}
	return errors.Join(errs...)
}

// Shutdown ends all live event streams (when the HTTP server starts draining).
func (m *Manager) Shutdown() {
	for _, rt := range m.Running() {
		rt.Hub.Close()
	}
}

// Create makes a new, empty soma for its owner: a new folder in SOMAS_DIR
// named after the soma (made unique), registered in the store.
func (m *Manager) Create(ctx context.Context, name string, ownerID int64) (store.Soma, error) {
	name, err := CleanName(name)
	if err != nil {
		return store.Soma{}, err
	}
	base := folderName(name)
	for i := 1; i < 1000; i++ {
		folder := base
		if i > 1 {
			folder = fmt.Sprintf("%s-%d", base, i)
		}
		err := os.Mkdir(filepath.Join(m.opts.SomasDir, folder), 0o755)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return store.Soma{}, err
		}
		s, err := m.store.CreateSoma(ctx, store.NewSomaID(), name, folder, ownerID)
		if errors.Is(err, store.ErrTaken) {
			continue // registered elsewhere already (unlikely): try the next name
		}
		if err != nil {
			os.Remove(filepath.Join(m.opts.SomasDir, folder))
			return store.Soma{}, err
		}
		return s, nil
	}
	return store.Soma{}, errors.New("no free folder name")
}

// Import registers an existing folder (outside SOMAS_DIR) as a soma.
func (m *Manager) Import(ctx context.Context, dir, name string, ownerID int64) (store.Soma, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return store.Soma{}, err
	}
	return m.store.CreateSoma(ctx, store.NewSomaID(), name, abs, ownerID)
}

// Delete takes a soma out of service and unregisters it. Its folder moves to
// SOMAS_DIR/.trash/ (an imported folder outside SOMAS_DIR stays where it is);
// nothing is deleted for good.
func (m *Manager) Delete(ctx context.Context, s store.Soma) error {
	if err := m.closeOne(s.ID); err != nil {
		m.opts.Log.Error("closing soma failed", "soma", s.ID, "err", err)
	}
	if !filepath.IsAbs(s.Folder) {
		trash := filepath.Join(m.opts.SomasDir, TrashDir)
		if err := os.MkdirAll(trash, 0o755); err != nil {
			return err
		}
		dst := filepath.Join(trash, fmt.Sprintf("%s-%s", s.Folder, time.Now().UTC().Format("20060102-150405")))
		if err := os.Rename(m.Path(s), dst); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := m.store.DeleteSoma(ctx, s.ID); err != nil {
		return err
	}
	_ = os.RemoveAll(m.indexDir(s.ID))
	return nil
}

// CleanName checks a soma's display name.
func CleanName(name string) (string, error) {
	name = strings.TrimSpace(norm.NFC.String(name))
	if name == "" {
		return "", errors.New("a soma needs a name")
	}
	if len(name) > 100 {
		return "", errors.New("soma names have at most 100 characters")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", errors.New("soma names cannot contain control characters")
		}
	}
	return name, nil
}

// folderName derives a folder name from a soma name: lower case letters,
// digits and dashes.
func folderName(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(norm.NFKD.String(name)) {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case unicode.Is(unicode.Mn, r):
			// accents dropped: é → e
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "soma"
	}
	return out
}
