package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxSessions  = 100
	refreshEvery = time.Hour // persist sliding expiry at most this often
	storeVersion = 1
)

// Session is a logged-in browser.
type Session struct {
	User    string    `json:"user"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
}

// Store keeps sessions in memory and persists them to a JSON file in
// DATA_DIR, so a restart does not log everybody out. Only SHA-256 hashes of
// the tokens are stored – a leaked file cannot be used to log in.
// Losing the file only means logging in again.
type Store struct {
	mu       sync.Mutex
	path     string
	ttl      time.Duration
	sessions map[string]*Session // key: hex(sha256(token))
	now      func() time.Time
}

type storeFile struct {
	Version  int                 `json:"version"`
	Sessions map[string]*Session `json:"sessions"`
}

// OpenStore loads the sessions file at path. A missing or unreadable file
// starts an empty store.
func OpenStore(path string, ttl time.Duration) (*Store, error) {
	s := &Store{path: path, ttl: ttl, sessions: map[string]*Session{}, now: time.Now}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		var f storeFile
		if json.Unmarshal(data, &f) == nil && f.Version == storeVersion && f.Sessions != nil {
			s.sessions = f.Sessions
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	return s, nil
}

func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create starts a new session and returns its token.
func (s *Store) Create(user string) (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	for len(s.sessions) >= maxSessions {
		s.evictOldestLocked()
	}
	now := s.now()
	s.sessions[tokenKey(token)] = &Session{User: user, Created: now, Expires: now.Add(s.ttl)}
	return token, s.saveLocked()
}

// Lookup returns the session for token and extends its expiry (sliding).
func (s *Store) Lookup(token string) (Session, bool) {
	if token == "" || len(token) > 128 {
		return Session{}, false
	}
	key := tokenKey(token)

	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[key]
	if !ok {
		return Session{}, false
	}
	now := s.now()
	if !now.Before(sess.Expires) {
		delete(s.sessions, key)
		_ = s.saveLocked()
		return Session{}, false
	}
	if next := now.Add(s.ttl); next.Sub(sess.Expires) >= refreshEvery {
		sess.Expires = next
		_ = s.saveLocked()
	}
	return *sess, true
}

// Delete ends the session for token.
func (s *Store) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := tokenKey(token)
	if _, ok := s.sessions[key]; !ok {
		return nil
	}
	delete(s.sessions, key)
	return s.saveLocked()
}

// Len returns the number of active sessions.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *Store) pruneLocked() {
	now := s.now()
	for k, sess := range s.sessions {
		if sess == nil || !now.Before(sess.Expires) {
			delete(s.sessions, k)
		}
	}
}

func (s *Store) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	for k, sess := range s.sessions {
		if oldestKey == "" || sess.Created.Before(oldest) {
			oldestKey, oldest = k, sess.Created
		}
	}
	delete(s.sessions, oldestKey)
}

// saveLocked writes the sessions file atomically with mode 0600.
func (s *Store) saveLocked() error {
	data, err := json.Marshal(storeFile{Version: storeVersion, Sessions: s.sessions})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".sessions-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	serr := tmp.Sync()
	cerr := tmp.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
