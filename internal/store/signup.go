package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Setting reads a server-wide setting into v; ErrNotFound if it was never set.
func (s *Store) Setting(ctx context.Context, key string, v any) error {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}

// SetSetting stores a server-wide setting as JSON.
func (s *Store) SetSetting(ctx context.Context, key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, string(data))
	return err
}

// SignupSettings say whether people may create accounts themselves.
type SignupSettings struct {
	Open bool `json:"open"`
	// Domains are the e-mail domains allowed to sign up; empty means any.
	Domains []string `json:"domains"`
}

const signupKey = "signup"

// MaxSignupDomains bounds the list of allowed domains.
const MaxSignupDomains = 100

// Signup returns the sign-up settings (closed until an admin opens it).
func (s *Store) Signup(ctx context.Context) (SignupSettings, error) {
	var v SignupSettings
	err := s.Setting(ctx, signupKey, &v)
	if errors.Is(err, ErrNotFound) {
		return SignupSettings{Domains: []string{}}, nil
	}
	if v.Domains == nil {
		v.Domains = []string{}
	}
	return v, err
}

// SetSignup stores the sign-up settings.
func (s *Store) SetSignup(ctx context.Context, v SignupSettings) error {
	if v.Domains == nil {
		v.Domains = []string{}
	}
	return s.SetSetting(ctx, signupKey, v)
}

// Allows reports whether an e-mail address may sign up under these settings.
func (v SignupSettings) Allows(email string) bool {
	if len(v.Domains) == 0 {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range v.Domains {
		if strings.EqualFold(d, domain) {
			return true
		}
	}
	return false
}

// PendingSignup is a sign-up waiting for the confirmation of its address.
type PendingSignup struct {
	Username     string
	DisplayName  string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// AddPendingSignup stores a sign-up for ttl and returns the token of its
// confirmation link. An earlier one for the same address gives way.
func (s *Store) AddPendingSignup(ctx context.Context, p PendingSignup, ttl time.Duration) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	now := s.now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM pending_signups WHERE lower(email) = lower(?) OR expires_at <= ?`, p.Email, now.Unix()); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO pending_signups
		(token_hash, username, display_name, email, password_hash, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tokenHash(token), p.Username, p.DisplayName, p.Email, p.PasswordHash, now.Unix(), now.Add(ttl).Unix()); err != nil {
		return "", err
	}
	return token, tx.Commit()
}

// TakePendingSignup returns the sign-up behind a token and removes it (one
// use); ErrNotFound for unknown or expired tokens.
func (s *Store) TakePendingSignup(ctx context.Context, token string) (PendingSignup, error) {
	if len(token) < 32 || len(token) > 64 {
		return PendingSignup{}, ErrNotFound
	}
	var p PendingSignup
	var created, expires int64
	err := s.db.QueryRowContext(ctx, `DELETE FROM pending_signups WHERE token_hash = ?
		RETURNING username, display_name, email, password_hash, created_at, expires_at`, tokenHash(token)).
		Scan(&p.Username, &p.DisplayName, &p.Email, &p.PasswordHash, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return PendingSignup{}, ErrNotFound
	}
	if err != nil {
		return PendingSignup{}, err
	}
	p.CreatedAt, p.ExpiresAt = fromUnix(created), fromUnix(expires)
	if !s.now().Before(p.ExpiresAt) {
		return PendingSignup{}, ErrNotFound
	}
	return p, nil
}

// UsersByEmail returns the accounts with an e-mail address (case-insensitive).
func (s *Store) UsersByEmail(ctx context.Context, email string) ([]User, error) {
	if strings.TrimSpace(email) == "" {
		return nil, nil
	}
	return s.queryUsers(ctx, `SELECT `+userCols+` FROM users WHERE email != '' AND lower(email) = lower(?) ORDER BY id`, email)
}
