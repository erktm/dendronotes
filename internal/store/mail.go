package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ---------------------------------------------------------------------------
// Outbox: emails wait here until the relay took them.

// OutMail is a message in the outbox.
type OutMail struct {
	ID        int64
	Kind      string
	To        string
	ToName    string
	Subject   string
	Text      string
	HTML      string
	CreatedAt time.Time
	Attempts  int
	NextAt    time.Time
	LastError string
}

// EnqueueMail puts a message into the outbox, due at once.
func (s *Store) EnqueueMail(ctx context.Context, m OutMail) (int64, error) {
	now := s.unixNow()
	res, err := s.db.ExecContext(ctx, `INSERT INTO mail_outbox (kind, to_addr, to_name, subject, text, html, created_at, next_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, m.Kind, m.To, m.ToName, m.Subject, m.Text, m.HTML, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DueMails returns up to limit messages whose time has come, oldest first.
func (s *Store) DueMails(ctx context.Context, limit int) ([]OutMail, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, to_addr, to_name, subject, text, html, created_at, attempts, next_at, last_error
		FROM mail_outbox WHERE next_at <= ? ORDER BY id LIMIT ?`, s.unixNow(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutMail
	for rows.Next() {
		var m OutMail
		var created, next int64
		if err := rows.Scan(&m.ID, &m.Kind, &m.To, &m.ToName, &m.Subject, &m.Text, &m.HTML, &created, &m.Attempts, &next, &m.LastError); err != nil {
			return nil, err
		}
		m.CreatedAt, m.NextAt = time.Unix(created, 0).UTC(), time.Unix(next, 0).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// MailDone removes a message from the outbox (sent, or given up).
func (s *Store) MailDone(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM mail_outbox WHERE id = ?`, id)
	return err
}

// MailRetry records a failed attempt and when to try again.
func (s *Store) MailRetry(ctx context.Context, id int64, next time.Time, errText string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE mail_outbox SET attempts = attempts + 1, next_at = ?, last_error = ? WHERE id = ?`,
		next.Unix(), errText, id)
	return err
}

// MailQueue reports how many messages wait and the last error among them.
func (s *Store) MailQueue(ctx context.Context) (waiting int, lastError string, err error) {
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mail_outbox`).Scan(&waiting); err != nil {
		return 0, "", err
	}
	err = s.db.QueryRowContext(ctx, `SELECT last_error FROM mail_outbox WHERE last_error <> '' ORDER BY next_at DESC LIMIT 1`).Scan(&lastError)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return waiting, lastError, err
}

// ---------------------------------------------------------------------------
// Password tokens: links to reset a password or to choose the first one.

// Purposes of password tokens.
const (
	TokenReset   = "reset"
	TokenWelcome = "welcome"
)

// CreatePasswordToken stores the hash of a new token for an account; older
// unused tokens of the same purpose stop working.
func (s *Store) CreatePasswordToken(ctx context.Context, userID int64, purpose, hash string, ttl time.Duration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.unixNow()
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_tokens WHERE user_id = ? AND purpose = ? AND used_at = 0`, userID, purpose); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO password_tokens (user_id, token_hash, purpose, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`, userID, hash, purpose, now, now+int64(ttl/time.Second)); err != nil {
		return err
	}
	return tx.Commit()
}

// LastPasswordToken returns when the newest token of an account and purpose
// was made (zero if none).
func (s *Store) LastPasswordToken(ctx context.Context, userID int64, purpose string) (time.Time, error) {
	var at sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(created_at) FROM password_tokens WHERE user_id = ? AND purpose = ?`, userID, purpose).Scan(&at)
	if err != nil || !at.Valid {
		return time.Time{}, err
	}
	return time.Unix(at.Int64, 0).UTC(), nil
}

// UsePasswordToken spends a token: it works once and only before it
// expires. ErrNotFound for unknown, used or expired tokens.
func (s *Store) UsePasswordToken(ctx context.Context, hash string) (userID int64, purpose string, err error) {
	now := s.unixNow()
	err = s.db.QueryRowContext(ctx, `UPDATE password_tokens SET used_at = ?1
		WHERE token_hash = ?2 AND used_at = 0 AND expires_at > ?1 RETURNING user_id, purpose`, now, hash).Scan(&userID, &purpose)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return userID, purpose, err
}

// DropPasswordTokens removes the unused tokens of an account (after its
// password changed some other way).
func (s *Store) DropPasswordTokens(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM password_tokens WHERE user_id = ? AND used_at = 0`, userID)
	return err
}

// CleanPasswordTokens forgets tokens that expired or were used a day ago.
func (s *Store) CleanPasswordTokens(ctx context.Context) error {
	cut := s.unixNow() - 24*3600
	_, err := s.db.ExecContext(ctx, `DELETE FROM password_tokens WHERE expires_at < ?1 OR (used_at <> 0 AND used_at < ?1)`, cut)
	return err
}

// ---------------------------------------------------------------------------
// Comment digests: unread notifications become an email after a while.

// DigestEntry is one unread notification due for email.
type DigestEntry struct {
	NotificationID int64
	UserID         int64
	Kind           string
	SomaID         string
	SomaName       string
	Path           string
	CommentID      int64
	Body           string
	Actor          string // display name, else user name
}

// DueDigests returns the unread notifications created up to before that
// were not handled for email yet, for accounts with an address that want
// them, under the same visibility rules as the notification list.
func (s *Store) DueDigests(ctx context.Context, before time.Time) ([]DigestEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT n.id, n.user_id, n.kind, n.soma_id, sm.name, n.path, n.comment_id, c.body,
		COALESCE(NULLIF(a.display_name, ''), a.username, '')
		FROM notifications n
		JOIN users u ON u.id = n.user_id
		JOIN comments c ON c.id = n.comment_id
		JOIN somas sm ON sm.id = n.soma_id
		LEFT JOIN users a ON a.id = n.actor_id
		WHERE n.read_at = 0 AND n.mailed_at = 0 AND n.created_at <= ?
		AND u.disabled = 0 AND u.email <> '' AND u.mail_comments = 1
		AND substr(n.path, 1, 7) <> '.trash/'
		AND EXISTS (SELECT 1 FROM soma_access x WHERE x.soma_id = n.soma_id AND x.user_id = n.user_id)
		ORDER BY n.user_id, n.created_at, n.id`, before.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DigestEntry
	for rows.Next() {
		var e DigestEntry
		if err := rows.Scan(&e.NotificationID, &e.UserID, &e.Kind, &e.SomaID, &e.SomaName, &e.Path, &e.CommentID, &e.Body, &e.Actor); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DigestsHandled marks every notification created up to before as handled
// for email, whether it was sent, read meanwhile or not wanted.
func (s *Store) DigestsHandled(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notifications SET mailed_at = ? WHERE mailed_at = 0 AND created_at <= ?`,
		s.unixNow(), before.Unix())
	return err
}
