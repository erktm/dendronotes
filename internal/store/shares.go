package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

// Kinds of share links.
const (
	LinkInternal = "internal"
	LinkPublic   = "public"
)

// MaxPublicLinks is how many public links a soma keeps.
const MaxPublicLinks = 500

// ErrTooManyLinks is returned when a soma has MaxPublicLinks public links.
var ErrTooManyLinks = errors.New("too many public links")

// ShareLink is a link to a note or drawing that follows it when it moves.
// Internal links open it for everyone with access; public links show it to
// anyone with the token (only its SHA-256 is kept).
type ShareLink struct {
	ID        string    `json:"id"`
	SomaID    string    `json:"-"`
	Path      string    `json:"path"`
	Kind      string    `json:"kind"`
	CreatedBy *Person   `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// ExpiresAt is zero for links that do not expire.
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
}

// Expired reports whether a public link has run out at now.
func (l ShareLink) Expired(now time.Time) bool {
	return !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt)
}

const linkCols = `l.id, l.soma_id, l.path, l.kind, l.created_at, l.expires_at, u.id, u.username, u.display_name`

func scanLink(row interface{ Scan(...any) error }) (ShareLink, error) {
	var l ShareLink
	var created, expires int64
	var uid sql.NullInt64
	var uname, dname sql.NullString
	if err := row.Scan(&l.ID, &l.SomaID, &l.Path, &l.Kind, &created, &expires, &uid, &uname, &dname); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ShareLink{}, ErrNotFound
		}
		return ShareLink{}, err
	}
	l.CreatedAt, l.ExpiresAt = fromUnix(created), fromUnix(expires)
	if uid.Valid {
		l.CreatedBy = &Person{ID: uid.Int64, Username: uname.String, DisplayName: dname.String}
	}
	return l, nil
}

const linkFrom = ` FROM share_links l LEFT JOIN users u ON u.id = l.created_by `

// newLinkID is a random id for a link (10 characters, like soma ids).
func newLinkID() string { return NewSomaID() }

// InternalLink returns the internal link of an entry, made on first use.
func (s *Store) InternalLink(ctx context.Context, somaID, path string, by int64) (ShareLink, error) {
	var byID any
	if by != 0 {
		byID = by
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO share_links (id, soma_id, path, kind, created_by, created_at)
		VALUES (?, ?, ?, 'internal', ?, ?) ON CONFLICT DO NOTHING`, newLinkID(), somaID, path, byID, s.unixNow())
	if err != nil {
		return ShareLink{}, err
	}
	return scanLink(s.db.QueryRowContext(ctx, `SELECT `+linkCols+linkFrom+`WHERE l.soma_id = ? AND l.path = ? AND l.kind = 'internal'`, somaID, path))
}

// Link returns a link of a soma by its id.
func (s *Store) Link(ctx context.Context, somaID, id string) (ShareLink, error) {
	return scanLink(s.db.QueryRowContext(ctx, `SELECT `+linkCols+linkFrom+`WHERE l.soma_id = ? AND l.id = ?`, somaID, id))
}

// CreatePublicLink makes a public link to an entry and returns its token,
// which is shown only now; expires zero means it does not expire.
func (s *Store) CreatePublicLink(ctx context.Context, somaID, path string, by int64, expires time.Time) (string, ShareLink, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM share_links WHERE soma_id = ? AND kind = 'public'`, somaID).Scan(&n); err != nil {
		return "", ShareLink{}, err
	}
	if n >= MaxPublicLinks {
		return "", ShareLink{}, ErrTooManyLinks
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", ShareLink{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	var exp int64
	if !expires.IsZero() {
		exp = expires.Unix()
	}
	var byID any
	if by != 0 {
		byID = by
	}
	id := newLinkID()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO share_links (id, soma_id, path, kind, token_hash, created_by, created_at, expires_at)
		VALUES (?, ?, ?, 'public', ?, ?, ?, ?)`, id, somaID, path, tokenHash(token), byID, s.unixNow(), exp); err != nil {
		return "", ShareLink{}, err
	}
	l, err := s.Link(ctx, somaID, id)
	return token, l, err
}

// PublicLink returns the public link behind a token (expired or not).
func (s *Store) PublicLink(ctx context.Context, token string) (ShareLink, error) {
	if len(token) < 32 || len(token) > 64 {
		return ShareLink{}, ErrNotFound
	}
	return scanLink(s.db.QueryRowContext(ctx, `SELECT `+linkCols+linkFrom+`WHERE l.token_hash = ? AND l.kind = 'public'`, tokenHash(token)))
}

// PublicLinks lists a soma's public links, newest first.
func (s *Store) PublicLinks(ctx context.Context, somaID string) ([]ShareLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+linkCols+linkFrom+`WHERE l.soma_id = ? AND l.kind = 'public'
		ORDER BY l.created_at DESC, l.rowid DESC`, somaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ShareLink{}
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DeletePublicLink revokes a public link.
func (s *Store) DeletePublicLink(ctx context.Context, somaID, id string) (ShareLink, error) {
	l, err := s.Link(ctx, somaID, id)
	if err != nil {
		return ShareLink{}, err
	}
	if l.Kind != LinkPublic {
		return ShareLink{}, ErrNotFound
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM share_links WHERE soma_id = ? AND id = ?`, somaID, id)
	return l, err
}

// MoveShareLinks lets the links to from (a note, or everything below a
// folder) follow it to to.
func (s *Store) MoveShareLinks(ctx context.Context, somaID, from, to string) error {
	if from == to {
		return nil
	}
	// An internal link already at the target (an earlier entry of that name)
	// gives way to the one that moves there.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM share_links WHERE soma_id = ?1 AND kind = 'internal'
		AND (path = ?3 OR substr(path, 1, length(?3) + 1) = ?3 || '/')
		AND EXISTS (SELECT 1 FROM share_links m WHERE m.soma_id = ?1 AND m.kind = 'internal'
			AND m.path = ?2 || substr(share_links.path, length(?3) + 1))`, somaID, from, to); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE share_links SET path = ?3 || substr(path, length(?2) + 1)
		WHERE soma_id = ?1 AND `+underPath, somaID, from, to); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveShareLinks deletes the links to p and below it; an empty path means
// the whole soma.
func (s *Store) RemoveShareLinks(ctx context.Context, somaID, p string) error {
	cond := `soma_id = ?1 AND ` + underPath
	if p == "" {
		cond = `soma_id = ?1 AND ?2 = ''`
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM share_links WHERE `+cond, somaID, p)
	return err
}
