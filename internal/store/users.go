package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Role of an account on the server.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// Source says how an account signs in.
type Source string

const (
	SourceLocal Source = "local" // password stored here
	SourceOIDC  Source = "oidc"  // OpenID Connect provider (Keycloak)
	SourceProxy Source = "proxy" // header from an authenticating proxy
)

// User is an account.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"displayName"`
	Email        string    `json:"email"`
	Role         Role      `json:"role"`
	Source       Source    `json:"source"`
	PasswordHash string    `json:"-"`
	OIDCIssuer   string    `json:"-"`
	OIDCSubject  string    `json:"-"`
	Disabled     bool      `json:"disabled"`
	CreatedAt    time.Time `json:"createdAt"`
	LastLoginAt  time.Time `json:"lastLoginAt"`
}

// Name is what the UI shows: the display name, else the user name.
func (u User) Name() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

const userCols = `id, username, display_name, email, role, source, password_hash, oidc_issuer, oidc_subject, disabled, created_at, last_login_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var (
		u              User
		created, login int64
	)
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Source, &u.PasswordHash,
		&u.OIDCIssuer, &u.OIDCSubject, &u.Disabled, &created, &login)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	u.CreatedAt, u.LastLoginAt = fromUnix(created), fromUnix(login)
	return u, err
}

// CreateUser adds an account; ErrTaken if the user name (case-insensitive)
// or the OIDC identity exists already.
func (s *Store) CreateUser(ctx context.Context, u User) (User, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO users
		(username, display_name, email, role, source, password_hash, oidc_issuer, oidc_subject, disabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.DisplayName, u.Email, u.Role, u.Source, u.PasswordHash, u.OIDCIssuer, u.OIDCSubject, u.Disabled, s.unixNow())
	if isUnique(err) {
		return User{}, ErrTaken
	}
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return s.UserByID(ctx, id)
}

// UserByID returns an account.
func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// UserByUsername returns an account by its user name (case-insensitive).
func (s *Store) UserByUsername(ctx context.Context, name string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username = ?`, name))
}

// UserByOIDC returns the account of an OpenID Connect identity.
func (s *Store) UserByOIDC(ctx context.Context, issuer, subject string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users
		WHERE source = 'oidc' AND oidc_issuer = ? AND oidc_subject = ?`, issuer, subject))
}

// Users lists all accounts by user name.
func (s *Store) Users(ctx context.Context) ([]User, error) {
	return s.queryUsers(ctx, `SELECT `+userCols+` FROM users ORDER BY username COLLATE NOCASE`)
}

// SearchUsers finds enabled accounts whose user or display name starts with q.
func (s *Store) SearchUsers(ctx context.Context, q string, limit int) ([]User, error) {
	pattern := escapeLike(strings.ToLower(q)) + "%"
	return s.queryUsers(ctx, `SELECT `+userCols+` FROM users
		WHERE disabled = 0 AND (lower(username) LIKE ? ESCAPE '\' OR lower(display_name) LIKE ? ESCAPE '\')
		ORDER BY username COLLATE NOCASE LIMIT ?`, pattern, pattern, limit)
}

func (s *Store) queryUsers(ctx context.Context, q string, args ...any) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UserUpdate changes selected fields of an account (nil: keep).
type UserUpdate struct {
	DisplayName  *string
	Email        *string
	Role         *Role
	Disabled     *bool
	PasswordHash *string
	Username     *string
}

// UpdateUser applies the changes; ErrNotFound for unknown accounts.
func (s *Store) UpdateUser(ctx context.Context, id int64, up UserUpdate) (User, error) {
	var (
		sets []string
		args []any
	)
	add := func(col string, v any) { sets = append(sets, col+" = ?"); args = append(args, v) }
	if up.DisplayName != nil {
		add("display_name", *up.DisplayName)
	}
	if up.Email != nil {
		add("email", *up.Email)
	}
	if up.Role != nil {
		add("role", *up.Role)
	}
	if up.Disabled != nil {
		add("disabled", *up.Disabled)
	}
	if up.PasswordHash != nil {
		add("password_hash", *up.PasswordHash)
	}
	if up.Username != nil {
		add("username", *up.Username)
	}
	if len(sets) > 0 {
		res, err := s.db.ExecContext(ctx, `UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, append(args, id)...)
		if isUnique(err) {
			return User{}, ErrTaken
		}
		if err != nil {
			return User{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return User{}, ErrNotFound
		}
	}
	return s.UserByID(ctx, id)
}

// TouchLogin records a successful sign-in.
func (s *Store) TouchLogin(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, s.unixNow(), id)
	return err
}

// DeleteUser removes an account with its sessions and memberships. Somas it
// owns must have been transferred or deleted before.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountAdmins counts enabled admin accounts.
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0`).Scan(&n)
	return n, err
}

// --- Sessions --------------------------------------------------------------

// Session is a signed-in browser.
type Session struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	SeenAt    time.Time `json:"seenAt"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"userAgent"`
}

// refreshEvery limits how often a session's expiry slides forward (writes).
const refreshEvery = 10 * time.Minute

// MaxSessionsPerUser bounds the sessions of one account; the oldest go first.
const MaxSessionsPerUser = 50

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession starts a session and returns its secret token (256 bits).
// Only a hash of the token is stored.
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration, ip, userAgent string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	now := s.unixNow()
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id NOT IN
		(SELECT id FROM sessions WHERE user_id = ? ORDER BY seen_at DESC LIMIT ?)`, userID, userID, MaxSessionsPerUser-1); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, expires_at, seen_at, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, tokenHash(token), userID, now, now+int64(ttl.Seconds()), now, ip, userAgent); err != nil {
		return "", err
	}
	return token, tx.Commit()
}

// LookupSession returns the account behind a session token if the session is
// valid and the account enabled; it slides the expiry forward (at most every
// refreshEvery, to keep writes rare).
func (s *Store) LookupSession(ctx context.Context, token string, ttl time.Duration) (User, int64, bool) {
	if token == "" || len(token) > 128 {
		return User{}, 0, false
	}
	var (
		sid           int64
		expires, seen int64
	)
	row := s.db.QueryRowContext(ctx, `SELECT s.id, s.expires_at, s.seen_at, `+prefixed("u", userCols)+`
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ?`, tokenHash(token))
	var (
		u              User
		created, login int64
	)
	err := row.Scan(&sid, &expires, &seen, &u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Source,
		&u.PasswordHash, &u.OIDCIssuer, &u.OIDCSubject, &u.Disabled, &created, &login)
	if err != nil {
		return User{}, 0, false
	}
	u.CreatedAt, u.LastLoginAt = fromUnix(created), fromUnix(login)
	now := s.unixNow()
	if now >= expires || u.Disabled {
		return User{}, 0, false
	}
	if time.Duration(now-seen)*time.Second >= refreshEvery {
		_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET seen_at = ?, expires_at = ? WHERE id = ?`, now, now+int64(ttl.Seconds()), sid)
	}
	return u, sid, true
}

// DeleteSession ends the session of a token.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash(token))
	return err
}

// DeleteSessionByID ends one session (admin).
func (s *Store) DeleteSessionByID(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUserSessions ends all sessions of an account, except keep (0: none).
func (s *Store) DeleteUserSessions(ctx context.Context, userID, keep int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id != ?`, userID, keep)
	return err
}

// Sessions lists the valid sessions, newest activity first.
func (s *Store) Sessions(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.user_id, u.username, s.created_at, s.expires_at, s.seen_at, s.ip, s.user_agent
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.expires_at > ? ORDER BY s.seen_at DESC`, s.unixNow())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var (
			x                      Session
			created, expires, seen int64
		)
		if err := rows.Scan(&x.ID, &x.UserID, &x.Username, &created, &expires, &seen, &x.IP, &x.UserAgent); err != nil {
			return nil, err
		}
		x.CreatedAt, x.ExpiresAt, x.SeenAt = fromUnix(created), fromUnix(expires), fromUnix(seen)
		out = append(out, x)
	}
	return out, rows.Err()
}

// prefixed qualifies a column list with a table alias.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ", ")
	for i, p := range parts {
		parts[i] = alias + "." + p
	}
	return strings.Join(parts, ", ")
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
