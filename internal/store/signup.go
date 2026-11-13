package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

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
