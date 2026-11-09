package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Comment is a comment on a note. A thread starts with a comment whose
// ThreadID is 0; replies carry the id of that first comment. The anchor
// (Quote with a little text before and after it) is where in the note the
// thread belongs; the client finds it in the current text.
type Comment struct {
	ID         int64     `json:"id"`
	SomaID     string    `json:"-"`
	Path       string    `json:"path"`
	ThreadID   int64     `json:"threadId"`
	AuthorID   int64     `json:"authorId"` // 0: the account is gone
	Body       string    `json:"body"`
	Quote      string    `json:"quote"`
	Prefix     string    `json:"prefix"`
	Suffix     string    `json:"suffix"`
	CreatedAt  time.Time `json:"createdAt"`
	EditedAt   time.Time `json:"editedAt,omitzero"`
	ResolvedAt time.Time `json:"resolvedAt,omitzero"`
	ResolvedBy int64     `json:"resolvedBy,omitempty"`
}

// Root is the id of the thread the comment belongs to.
func (c Comment) Root() int64 {
	if c.ThreadID != 0 {
		return c.ThreadID
	}
	return c.ID
}

const commentCols = `id, soma_id, path, thread_id, COALESCE(author_id, 0), body, quote, prefix, suffix,
	created_at, edited_at, resolved_at, COALESCE(resolved_by, 0)`

func unixOrZero(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}

func scanComment(row interface{ Scan(...any) error }) (Comment, error) {
	var c Comment
	var created, edited, resolved int64
	err := row.Scan(&c.ID, &c.SomaID, &c.Path, &c.ThreadID, &c.AuthorID, &c.Body, &c.Quote, &c.Prefix, &c.Suffix,
		&created, &edited, &resolved, &c.ResolvedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	c.CreatedAt, c.EditedAt, c.ResolvedAt = time.Unix(created, 0).UTC(), unixOrZero(edited), unixOrZero(resolved)
	return c, err
}

// AddComment stores a new comment (a thread, or a reply when ThreadID is set).
func (s *Store) AddComment(ctx context.Context, c Comment) (Comment, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO comments (soma_id, path, thread_id, author_id, body, quote, prefix, suffix, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, c.SomaID, c.Path, c.ThreadID, c.AuthorID, c.Body, c.Quote, c.Prefix, c.Suffix, s.unixNow())
	if err != nil {
		return Comment{}, err
	}
	id, _ := res.LastInsertId()
	return s.CommentByID(ctx, c.SomaID, id)
}

// CommentByID returns a comment of a soma.
func (s *Store) CommentByID(ctx context.Context, somaID string, id int64) (Comment, error) {
	return scanComment(s.db.QueryRowContext(ctx, `SELECT `+commentCols+` FROM comments WHERE soma_id = ? AND id = ?`, somaID, id))
}

// Comments lists the comments on a note, oldest first.
func (s *Store) Comments(ctx context.Context, somaID, path string) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+commentCols+` FROM comments WHERE soma_id = ? AND path = ? ORDER BY created_at, id`, somaID, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ThreadAuthors lists everyone who wrote in a thread.
func (s *Store) ThreadAuthors(ctx context.Context, somaID string, root int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT author_id FROM comments
		WHERE soma_id = ? AND (id = ?2 OR thread_id = ?2) AND author_id IS NOT NULL`, somaID, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// EditComment replaces the text of a comment.
func (s *Store) EditComment(ctx context.Context, somaID string, id int64, body string) (Comment, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE comments SET body = ?, edited_at = ? WHERE soma_id = ? AND id = ?`, body, s.unixNow(), somaID, id)
	if err != nil {
		return Comment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Comment{}, ErrNotFound
	}
	return s.CommentByID(ctx, somaID, id)
}

// ResolveThread marks a thread as done (by) or open again (by 0).
func (s *Store) ResolveThread(ctx context.Context, somaID string, root, by int64) (Comment, error) {
	var at, who any
	if by != 0 {
		at, who = s.unixNow(), by
	} else {
		at = 0
	}
	res, err := s.db.ExecContext(ctx, `UPDATE comments SET resolved_at = ?, resolved_by = ? WHERE soma_id = ? AND id = ? AND thread_id = 0`,
		at, who, somaID, root)
	if err != nil {
		return Comment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Comment{}, ErrNotFound
	}
	return s.CommentByID(ctx, somaID, root)
}

// DeleteComment deletes a comment – with its replies when it starts a
// thread – and the notifications about them.
func (s *Store) DeleteComment(ctx context.Context, somaID string, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	const which = `soma_id = ?1 AND (id = ?2 OR thread_id = ?2)`
	if _, err := tx.ExecContext(ctx, `DELETE FROM notifications WHERE comment_id IN (SELECT id FROM comments WHERE `+which+`)`, somaID, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE `+which, somaID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// underPath matches path and everything below it (path as a folder).
const underPath = `(path = ?2 OR substr(path, 1, length(?2) + 1) = ?2 || '/')`

// MoveComments lets the comments on from (a note, or everything below a
// folder) follow it to to.
func (s *Store) MoveComments(ctx context.Context, somaID, from, to string) error {
	if from == to {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"comments", "notifications"} {
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET path = ?3 || substr(path, length(?2) + 1) WHERE soma_id = ?1 AND `+underPath,
			somaID, from, to); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveComments deletes the comments on path (and below it) with their
// notifications; an empty path means the whole soma.
func (s *Store) RemoveComments(ctx context.Context, somaID, path string) error {
	cond := `soma_id = ?1 AND ` + underPath
	if path == "" {
		cond = `soma_id = ?1 AND ?2 = ''`
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"notifications", "comments"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+cond, somaID, path); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SomaPeople lists the enabled accounts with access to a soma whose user
// name or display name starts with q (for mentions).
func (s *Store) SomaPeople(ctx context.Context, somaID, q string, limit int) ([]Person, error) {
	pattern := escapeLike(strings.ToLower(q)) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.username, u.display_name, u.avatar_style, u.avatar_photo FROM users u
		WHERE u.disabled = 0 AND EXISTS (SELECT 1 FROM soma_access a WHERE a.soma_id = ? AND a.user_id = u.id)
		AND (lower(u.username) LIKE ? ESCAPE '\' OR lower(u.display_name) LIKE ? ESCAPE '\')
		ORDER BY u.username COLLATE NOCASE LIMIT ?`, somaID, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		var p Person
		var style string
		var photo int64
		if err := rows.Scan(&p.ID, &p.Username, &p.DisplayName, &style, &photo); err != nil {
			return nil, err
		}
		a := avatarOf(p.ID, style, photo)
		p.Avatar = &a
		out = append(out, p)
	}
	return out, rows.Err()
}

// Notification tells an account that it was mentioned or answered.
type Notification struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"` // mention, reply
	SomaID    string    `json:"somaId"`
	SomaName  string    `json:"somaName"`
	Path      string    `json:"path"`
	CommentID int64     `json:"commentId"`
	ThreadID  int64     `json:"threadId"`
	Excerpt   string    `json:"excerpt"`
	Actor     *Person   `json:"actor"` // nil if the account is gone
	CreatedAt time.Time `json:"createdAt"`
	Read      bool      `json:"read"`
}

// Notification kinds.
const (
	NotifyMention = "mention"
	NotifyReply   = "reply"
)

// Notify stores a notification for each of users.
func (s *Store) Notify(ctx context.Context, kind string, users []int64, c Comment, actor int64) error {
	now := s.unixNow()
	for _, u := range users {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO notifications (user_id, kind, soma_id, path, comment_id, actor_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, u, kind, c.SomaID, c.Path, c.ID, actor, now); err != nil {
			return err
		}
	}
	return nil
}

// visibleNotifications are those about comments that still exist, on notes
// outside the trash, in somas the account can still open.
const visibleNotifications = `FROM notifications n
	JOIN comments c ON c.id = n.comment_id
	JOIN somas s ON s.id = n.soma_id
	LEFT JOIN users a ON a.id = n.actor_id
	WHERE n.user_id = ?1 AND substr(n.path, 1, 7) <> '.trash/'
	AND EXISTS (SELECT 1 FROM soma_access x WHERE x.soma_id = n.soma_id AND x.user_id = ?1)`

// Notifications returns the newest notifications of an account and how
// many are unread.
func (s *Store) Notifications(ctx context.Context, userID int64, limit int) ([]Notification, int, error) {
	var unread int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) `+visibleNotifications+` AND n.read_at = 0`, userID).Scan(&unread); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT n.id, n.kind, n.soma_id, s.name, n.path, n.comment_id, c.thread_id, c.body,
		n.created_at, n.read_at, COALESCE(a.id, 0), COALESCE(a.username, ''), COALESCE(a.display_name, ''),
		COALESCE(a.avatar_style, ''), COALESCE(a.avatar_photo, 0)
		`+visibleNotifications+` ORDER BY n.created_at DESC, n.id DESC LIMIT ?2`, userID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var created, read int64
		var a Person
		var style string
		var photo int64
		if err := rows.Scan(&n.ID, &n.Kind, &n.SomaID, &n.SomaName, &n.Path, &n.CommentID, &n.ThreadID, &n.Excerpt,
			&created, &read, &a.ID, &a.Username, &a.DisplayName, &style, &photo); err != nil {
			return nil, 0, err
		}
		if n.ThreadID == 0 {
			n.ThreadID = n.CommentID
		}
		if r := []rune(n.Excerpt); len(r) > 140 {
			n.Excerpt = string(r[:139]) + "…"
		}
		n.CreatedAt, n.Read = time.Unix(created, 0).UTC(), read != 0
		if a.ID != 0 {
			av := avatarOf(a.ID, style, photo)
			a.Avatar = &av
			n.Actor = &a
		}
		out = append(out, n)
	}
	return out, unread, rows.Err()
}

// MarkNotificationsRead marks the given notifications (all with nil) of an
// account as read.
func (s *Store) MarkNotificationsRead(ctx context.Context, userID int64, ids []int64) error {
	now := s.unixNow()
	if ids == nil {
		_, err := s.db.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at = 0`, now, userID)
		return err
	}
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND id = ? AND read_at = 0`, now, userID, id); err != nil {
			return err
		}
	}
	return nil
}
