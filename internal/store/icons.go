package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// MaxIconsPerSoma bounds the icon library of a soma.
const MaxIconsPerSoma = 300

// ErrTooManyIcons means the library is full.
var ErrTooManyIcons = errors.New("too many icons")

// Icon is a picture in the icon library of a soma (without its data).
type Icon struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int       `json:"size"`
	CreatedBy int64     `json:"createdBy"` // 0: unknown or gone
	CreatedAt time.Time `json:"createdAt"`
}

// AddIcon stores an icon (already checked and normalized).
func (s *Store) AddIcon(ctx context.Context, somaID, name, mime string, data []byte, by int64) (Icon, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM soma_icons WHERE soma_id = ?`, somaID).Scan(&n); err != nil {
		return Icon{}, err
	}
	if n >= MaxIconsPerSoma {
		return Icon{}, ErrTooManyIcons
	}
	var who any
	if by != 0 {
		who = by
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO soma_icons (soma_id, name, mime, data, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		somaID, name, mime, data, who, s.unixNow())
	if err != nil {
		return Icon{}, err
	}
	id, _ := res.LastInsertId()
	icon, _, err := s.IconByID(ctx, somaID, id)
	return icon, err
}

const iconCols = `id, name, mime, length(data), COALESCE(created_by, 0), created_at`

func scanIcon(row interface{ Scan(...any) error }, extra ...any) (Icon, error) {
	var i Icon
	var at int64
	err := row.Scan(append([]any{&i.ID, &i.Name, &i.Mime, &i.Size, &i.CreatedBy, &at}, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return i, ErrNotFound
	}
	i.CreatedAt = time.Unix(at, 0).UTC()
	return i, err
}

// Icons lists the icon library of a soma, by name.
func (s *Store) Icons(ctx context.Context, somaID string) ([]Icon, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+iconCols+` FROM soma_icons WHERE soma_id = ? ORDER BY name COLLATE NOCASE, id`, somaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Icon{}
	for rows.Next() {
		i, err := scanIcon(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// IconByID returns an icon of a soma with its data.
func (s *Store) IconByID(ctx context.Context, somaID string, id int64) (Icon, []byte, error) {
	var data []byte
	i, err := scanIcon(s.db.QueryRowContext(ctx, `SELECT `+iconCols+`, data FROM soma_icons WHERE soma_id = ? AND id = ?`, somaID, id), &data)
	return i, data, err
}

// DeleteIcon removes an icon from the library (drawings keep their copy).
func (s *Store) DeleteIcon(ctx context.Context, somaID string, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM soma_icons WHERE soma_id = ? AND id = ?`, somaID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
