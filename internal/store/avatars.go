package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Avatar is how an account appears next to its name: a designed picture
// (color, pattern, initials) and, optionally, an uploaded photo shown
// instead of it.
type Avatar struct {
	// Color is an index into the fixed avatar colors (0 … AvatarColors-1).
	Color int `json:"color"`
	// Pattern drawn behind the initials, one of AvatarPatterns.
	Pattern string `json:"pattern"`
	// Initials chosen by the user (up to two letters or digits); empty means
	// the UI derives them from the name.
	Initials string `json:"initials"`
	// Photo is the version of the uploaded photo (Unix milliseconds of the
	// upload), 0 when there is none.
	Photo int64 `json:"photo"`
}

// AvatarColors is the number of avatar colors.
const AvatarColors = 8

// AvatarPatterns are the patterns a designed avatar may use.
var AvatarPatterns = []string{"plain", "dendrite", "rings", "vesicles"}

// ErrInvalidAvatar reports a design outside the allowed values.
var ErrInvalidAvatar = errors.New("invalid avatar")

// AvatarStyle is the designed part of an avatar.
type AvatarStyle struct {
	Color    int    `json:"color"`
	Pattern  string `json:"pattern"`
	Initials string `json:"initials"`
}

// Normalize checks a design and returns it cleaned up: initials trimmed and
// upper-cased.
func (st AvatarStyle) Normalize() (AvatarStyle, error) {
	if st.Color < 0 || st.Color >= AvatarColors {
		return st, ErrInvalidAvatar
	}
	ok := false
	for _, p := range AvatarPatterns {
		ok = ok || st.Pattern == p
	}
	if !ok {
		return st, ErrInvalidAvatar
	}
	st.Initials = strings.ToUpper(strings.TrimSpace(st.Initials))
	if utf8.RuneCountInString(st.Initials) > 2 {
		return st, ErrInvalidAvatar
	}
	for _, r := range st.Initials {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return st, ErrInvalidAvatar
		}
	}
	return st, nil
}

// avatarOf builds the avatar of an account from its stored columns; accounts
// that never designed one get a color picked from their id.
func avatarOf(id int64, style string, photo int64) Avatar {
	a := Avatar{Color: int(id % AvatarColors), Pattern: "plain", Photo: photo}
	if style == "" {
		return a
	}
	var st AvatarStyle
	if json.Unmarshal([]byte(style), &st) != nil {
		return a
	}
	if st, err := st.Normalize(); err == nil {
		a.Color, a.Pattern, a.Initials = st.Color, st.Pattern, st.Initials
	}
	return a
}

// SetAvatarStyle stores the designed avatar of an account.
func (s *Store) SetAvatarStyle(ctx context.Context, id int64, st AvatarStyle) (User, error) {
	st, err := st.Normalize()
	if err != nil {
		return User{}, err
	}
	raw, _ := json.Marshal(st)
	res, err := s.db.ExecContext(ctx, `UPDATE users SET avatar_style = ? WHERE id = ?`, string(raw), id)
	if err != nil {
		return User{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return User{}, ErrNotFound
	}
	return s.UserByID(ctx, id)
}

// SetAvatarPhoto stores an uploaded photo (already checked and re-encoded by
// the caller) and gives it a new version.
func (s *Store) SetAvatarPhoto(ctx context.Context, id int64, image []byte) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var old int64
	if err := tx.QueryRowContext(ctx, `SELECT avatar_photo FROM users WHERE id = ?`, id).Scan(&old); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	version := max(s.now().UnixMilli(), old+1) // always changes, so caches notice
	if _, err := tx.ExecContext(ctx, `INSERT INTO avatars (user_id, image) VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET image = excluded.image`, id, image); err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET avatar_photo = ? WHERE id = ?`, version, id); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, id)
}

// RemoveAvatarPhoto deletes the photo of an account; the designed avatar
// shows again.
func (s *Store) RemoveAvatarPhoto(ctx context.Context, id int64) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM avatars WHERE user_id = ?`, id); err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET avatar_photo = 0 WHERE id = ?`, id); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, id)
}

// AvatarPhoto returns the photo of an account and its version; ErrNotFound
// when it has none.
func (s *Store) AvatarPhoto(ctx context.Context, id int64) ([]byte, int64, error) {
	var (
		image   []byte
		version int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT a.image, u.avatar_photo FROM avatars a JOIN users u ON u.id = a.user_id
		WHERE a.user_id = ?`, id).Scan(&image, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrNotFound
	}
	return image, version, err
}

// SharesSoma reports whether two accounts both have access to a common soma
// (personally, through a group or because it is shared with everyone).
func (s *Store) SharesSoma(ctx context.Context, a, b int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM soma_access x
		JOIN soma_access y ON y.soma_id = x.soma_id WHERE x.user_id = ? AND y.user_id = ?)`, a, b).Scan(&n)
	return n == 1, err
}
