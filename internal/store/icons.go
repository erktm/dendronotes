package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// An icon library is a soma's (library = the soma id) or the server's
// (library = ServerIcons), shown to everyone signed in. Icons can sit in
// sections (packs, or groups people make); those without one are plain
// uploads.

// ServerIcons is the library of server-wide icons, managed by admins.
const ServerIcons = ""

// MaxIcons bounds an icon library.
const MaxIcons = 2000

// MaxIconSections bounds the sections of a library.
const MaxIconSections = 100

// ErrTooManyIcons means the library is full.
var ErrTooManyIcons = errors.New("too many icons")

// Icon is a picture in an icon library (without its data).
type Icon struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int       `json:"size"`
	SectionID int64     `json:"sectionId,omitempty"` // 0: no section
	CreatedBy int64     `json:"createdBy"`           // 0: unknown or gone
	CreatedAt time.Time `json:"createdAt"`
}

// IconSection groups icons of a library.
type IconSection struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// AddIcon stores an icon (already checked and normalized), in a section of
// the same library or in none (section 0).
func (s *Store) AddIcon(ctx context.Context, library, name, mime string, data []byte, by, section int64) (Icon, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM soma_icons WHERE soma_id = ?`, library).Scan(&n); err != nil {
		return Icon{}, err
	}
	if n >= MaxIcons {
		return Icon{}, ErrTooManyIcons
	}
	sec, err := s.sectionRef(ctx, library, section)
	if err != nil {
		return Icon{}, err
	}
	var who any
	if by != 0 {
		who = by
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO soma_icons (soma_id, name, mime, data, created_by, created_at, section_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		library, name, mime, data, who, s.unixNow(), sec)
	if err != nil {
		return Icon{}, err
	}
	id, _ := res.LastInsertId()
	icon, _, err := s.IconByID(ctx, library, id)
	return icon, err
}

// sectionRef checks that a section belongs to the library (nil for none).
func (s *Store) sectionRef(ctx context.Context, library string, section int64) (any, error) {
	if section == 0 {
		return nil, nil
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM icon_sections WHERE id = ? AND soma_id = ?`, section, library).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return id, err
}

const iconCols = `id, name, mime, length(data), COALESCE(section_id, 0), COALESCE(created_by, 0), created_at`

func scanIcon(row interface{ Scan(...any) error }, extra ...any) (Icon, error) {
	var i Icon
	var at int64
	err := row.Scan(append([]any{&i.ID, &i.Name, &i.Mime, &i.Size, &i.SectionID, &i.CreatedBy, &at}, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return i, ErrNotFound
	}
	i.CreatedAt = time.Unix(at, 0).UTC()
	return i, err
}

// Icons lists an icon library, by name.
func (s *Store) Icons(ctx context.Context, library string) ([]Icon, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+iconCols+` FROM soma_icons WHERE soma_id = ? ORDER BY name COLLATE NOCASE, id`, library)
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

// IconByID returns an icon of a library with its data.
func (s *Store) IconByID(ctx context.Context, library string, id int64) (Icon, []byte, error) {
	var data []byte
	i, err := scanIcon(s.db.QueryRowContext(ctx, `SELECT `+iconCols+`, data FROM soma_icons WHERE soma_id = ? AND id = ?`, library, id), &data)
	return i, data, err
}

// DeleteIcon removes an icon from a library (drawings keep their copy).
func (s *Store) DeleteIcon(ctx context.Context, library string, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM soma_icons WHERE soma_id = ? AND id = ?`, library, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveIcon puts an icon into another section of its library (0: none).
func (s *Store) MoveIcon(ctx context.Context, library string, id, section int64) error {
	sec, err := s.sectionRef(ctx, library, section)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE soma_icons SET section_id = ? WHERE soma_id = ? AND id = ?`, sec, library, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// IconSections lists the sections of a library in their order.
func (s *Store) IconSections(ctx context.Context, library string) ([]IconSection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, position FROM icon_sections WHERE soma_id = ? ORDER BY position, id`, library)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IconSection{}
	for rows.Next() {
		var sec IconSection
		if err := rows.Scan(&sec.ID, &sec.Name, &sec.Position); err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	return out, rows.Err()
}

// IconSection returns the section of that name in a library, made at the end
// when there is none yet (names compare case-insensitively).
func (s *Store) IconSection(ctx context.Context, library, name string) (IconSection, error) {
	var sec IconSection
	err := s.db.QueryRowContext(ctx, `SELECT id, name, position FROM icon_sections WHERE soma_id = ? AND lower(name) = lower(?)`, library, name).
		Scan(&sec.ID, &sec.Name, &sec.Position)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return sec, err
	}
	return s.AddIconSection(ctx, library, name)
}

// AddIconSection makes a section at the end of a library.
func (s *Store) AddIconSection(ctx context.Context, library, name string) (IconSection, error) {
	var n, last int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(position), -1) FROM icon_sections WHERE soma_id = ?`, library).Scan(&n, &last); err != nil {
		return IconSection{}, err
	}
	if n >= MaxIconSections {
		return IconSection{}, ErrTooManyIcons
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO icon_sections (soma_id, name, position, created_at) VALUES (?, ?, ?, ?)`,
		library, strings.TrimSpace(name), last+1, s.unixNow())
	if err != nil {
		return IconSection{}, err
	}
	id, _ := res.LastInsertId()
	return IconSection{ID: id, Name: strings.TrimSpace(name), Position: last + 1}, nil
}

// RenameIconSection renames a section of a library.
func (s *Store) RenameIconSection(ctx context.Context, library string, id int64, name string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE icon_sections SET name = ? WHERE soma_id = ? AND id = ?`, strings.TrimSpace(name), library, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// OrderIconSections puts the sections of a library in the order of ids;
// sections not named keep their place after them.
func (s *Store) OrderIconSections(ctx context.Context, library string, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE icon_sections SET position = ? WHERE soma_id = ? AND id = ?`, i, library, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE icon_sections SET position = position + ? WHERE soma_id = ? AND id NOT IN (SELECT value FROM json_each(?))`,
		len(ids), library, int64sJSON(ids)); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteIconSection removes a section with its icons (drawings keep their copies).
func (s *Store) DeleteIconSection(ctx context.Context, library string, id int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM soma_icons WHERE soma_id = ? AND section_id = ?`, library, id)
	if err != nil {
		return 0, err
	}
	icons, _ := res.RowsAffected()
	res, err = tx.ExecContext(ctx, `DELETE FROM icon_sections WHERE soma_id = ? AND id = ?`, library, id)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	return icons, tx.Commit()
}

func int64sJSON(ids []int64) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconvInt(id))
	}
	b.WriteByte(']')
	return b.String()
}

func strconvInt(v int64) string { return strconv.FormatInt(v, 10) }
