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

// TokenPrefix starts every personal API token (easy to recognize in logs
// and secret scanners).
const TokenPrefix = "dn_"

// Token scopes.
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
)

// MaxTokensPerUser bounds the tokens of one account.
const MaxTokensPerUser = 20

// ErrTooManyTokens means the account has the maximum number of tokens.
var ErrTooManyTokens = errors.New("too many tokens")

// APIToken describes a personal API token (never the token itself).
type APIToken struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"-"`
	Name       string    `json:"name"`
	Scope      string    `json:"scope"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt,omitzero"` // zero: never
	LastUsedAt time.Time `json:"lastUsedAt,omitzero"`
	LastUsedIP string    `json:"lastUsedIp,omitempty"`
}

const tokenCols = `id, user_id, name, scope, created_at, expires_at, last_used_at, last_used_ip`

func scanToken(row interface{ Scan(...any) error }, extra ...any) (APIToken, error) {
	var t APIToken
	var created, expires, used int64
	err := row.Scan(append([]any{&t.ID, &t.UserID, &t.Name, &t.Scope, &created, &expires, &used, &t.LastUsedIP}, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	t.CreatedAt, t.ExpiresAt, t.LastUsedAt = time.Unix(created, 0).UTC(), unixOrZero(expires), unixOrZero(used)
	return t, err
}

// CreateToken makes a new token for an account and returns it once; only
// its hash is stored. A zero expires never expires.
func (s *Store) CreateToken(ctx context.Context, userID int64, name, scope string, expires time.Time) (string, APIToken, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_tokens WHERE user_id = ?`, userID).Scan(&n); err != nil {
		return "", APIToken{}, err
	}
	if n >= MaxTokensPerUser {
		return "", APIToken{}, ErrTooManyTokens
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", APIToken{}, err
	}
	token := TokenPrefix + base64.RawURLEncoding.EncodeToString(raw[:])
	var exp int64
	if !expires.IsZero() {
		exp = expires.Unix()
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens (user_id, name, token_hash, scope, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`, userID, name, tokenHash(token), scope, s.unixNow(), exp)
	if err != nil {
		return "", APIToken{}, err
	}
	id, _ := res.LastInsertId()
	t, err := scanToken(s.db.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE id = ?`, id))
	return token, t, err
}

// LookupToken returns the account and token behind a token if it is valid,
// not expired and the account enabled. It notes when and from where it was
// used (at most once a minute).
func (s *Store) LookupToken(ctx context.Context, token, ip string) (User, APIToken, bool) {
	if !strings.HasPrefix(token, TokenPrefix) || len(token) > 128 {
		return User{}, APIToken{}, false
	}
	var u User
	var created, login int64
	var style string
	var photo int64
	t, err := scanToken(s.db.QueryRowContext(ctx, `SELECT `+prefixed("t", tokenCols)+`, `+prefixed("u", userCols)+`
		FROM api_tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ?`, tokenHash(token)),
		&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Source, &u.PasswordHash, &u.OIDCIssuer, &u.OIDCSubject,
		&u.LDAPID, &u.Disabled, &created, &login, &style, &photo)
	if err != nil {
		return User{}, APIToken{}, false
	}
	u.CreatedAt, u.LastLoginAt = fromUnix(created), fromUnix(login)
	u.Avatar = avatarOf(u.ID, style, photo)
	now := s.unixNow()
	if u.Disabled || (!t.ExpiresAt.IsZero() && now >= t.ExpiresAt.Unix()) {
		return User{}, APIToken{}, false
	}
	if now-t.LastUsedAt.Unix() >= 60 || t.LastUsedIP != ip {
		_, _ = s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ?, last_used_ip = ? WHERE id = ?`, now, ip, t.ID)
	}
	return u, t, true
}

// Tokens lists the tokens of an account, newest first.
func (s *Store) Tokens(ctx context.Context, userID int64) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TokenCounts returns how many tokens each account has (accounts without
// tokens are missing).
func (s *Store) TokenCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id, COUNT(*) FROM api_tokens GROUP BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// DeleteToken revokes a token of an account.
func (s *Store) DeleteToken(ctx context.Context, userID, id int64) (APIToken, error) {
	t, err := scanToken(s.db.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE user_id = ? AND id = ?`, userID, id))
	if err != nil {
		return APIToken{}, err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	return t, err
}

// DeleteUserTokens revokes all tokens of an account and returns how many.
func (s *Store) DeleteUserTokens(ctx context.Context, userID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
