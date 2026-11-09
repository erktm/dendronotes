package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Group is a set of accounts a soma can be shared with. The built-in group
// Everyone has no stored members: it is every enabled account.
type Group struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Everyone bool   `json:"everyone"`
	// Members counts the accounts in the group (for Everyone: all enabled ones).
	Members   int       `json:"members"`
	CreatedAt time.Time `json:"createdAt"`
}

// GroupGrant is a group a soma is shared with.
type GroupGrant struct {
	Group
	Role string `json:"role"`
}

// ErrBuiltinGroup refuses changes to the Everyone group.
var ErrBuiltinGroup = errors.New("the Everyone group cannot be changed")

const groupCols = `g.id, g.name, g.everyone, g.created_at,
	CASE WHEN g.everyone = 1 THEN (SELECT COUNT(*) FROM users WHERE disabled = 0)
	ELSE (SELECT COUNT(*) FROM group_members m WHERE m.group_id = g.id) END`

func scanGroup(row interface{ Scan(...any) error }, extra ...any) (Group, error) {
	var (
		g       Group
		created int64
	)
	err := row.Scan(append([]any{&g.ID, &g.Name, &g.Everyone, &created, &g.Members}, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	g.CreatedAt = fromUnix(created)
	return g, err
}

func (s *Store) queryGroups(ctx context.Context, query string, args ...any) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Groups lists all groups, Everyone first, then by name.
func (s *Store) Groups(ctx context.Context) ([]Group, error) {
	return s.queryGroups(ctx, `SELECT `+groupCols+` FROM groups g ORDER BY g.everyone DESC, g.name COLLATE NOCASE`)
}

// GroupByID returns a group.
func (s *Store) GroupByID(ctx context.Context, id int64) (Group, error) {
	return scanGroup(s.db.QueryRowContext(ctx, `SELECT `+groupCols+` FROM groups g WHERE g.id = ?`, id))
}

// SearchGroups finds groups whose name starts with q (case-insensitive).
func (s *Store) SearchGroups(ctx context.Context, q string, limit int) ([]Group, error) {
	pattern := escapeLike(strings.ToLower(q)) + "%"
	return s.queryGroups(ctx, `SELECT `+groupCols+` FROM groups g WHERE lower(g.name) LIKE ? ESCAPE '\'
		ORDER BY g.everyone DESC, g.name COLLATE NOCASE LIMIT ?`, pattern, limit)
}

// CreateGroup adds a group; ErrTaken if the name (case-insensitive) exists.
func (s *Store) CreateGroup(ctx context.Context, name string) (Group, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO groups (name, created_at) VALUES (?, ?)`, name, s.unixNow())
	if isUnique(err) {
		return Group{}, ErrTaken
	}
	if err != nil {
		return Group{}, err
	}
	id, _ := res.LastInsertId()
	return s.GroupByID(ctx, id)
}

// changeable returns ErrNotFound for unknown groups and ErrBuiltinGroup for
// Everyone.
func (s *Store) changeable(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) error {
	var everyone bool
	err := q.QueryRowContext(ctx, `SELECT everyone FROM groups WHERE id = ?`, id).Scan(&everyone)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if everyone {
		return ErrBuiltinGroup
	}
	return nil
}

// RenameGroup changes a group's name.
func (s *Store) RenameGroup(ctx context.Context, id int64, name string) (Group, error) {
	if err := s.changeable(ctx, s.db, id); err != nil {
		return Group{}, err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE groups SET name = ? WHERE id = ?`, name, id)
	if isUnique(err) {
		return Group{}, ErrTaken
	}
	if err != nil {
		return Group{}, err
	}
	return s.GroupByID(ctx, id)
}

// DeleteGroup removes a group, its members and the somas shared with it.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	if err := s.changeable(ctx, s.db, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM groups WHERE id = ?`, id)
	return err
}

// GroupMembers lists the accounts in a group (not for Everyone).
func (s *Store) GroupMembers(ctx context.Context, id int64) ([]Person, error) {
	if err := s.changeable(ctx, s.db, id); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.username, u.display_name, u.avatar_style, u.avatar_photo
		FROM group_members m JOIN users u ON u.id = m.user_id WHERE m.group_id = ?
		ORDER BY u.username COLLATE NOCASE`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		var (
			p     Person
			style string
			photo int64
		)
		if err := rows.Scan(&p.ID, &p.Username, &p.DisplayName, &style, &photo); err != nil {
			return nil, err
		}
		a := avatarOf(p.ID, style, photo)
		p.Avatar = &a
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddGroupMember puts an account into a group (nothing happens if it is in
// already); ErrNotFound for an unknown account.
func (s *Store) AddGroupMember(ctx context.Context, groupID, userID int64) error {
	if err := s.changeable(ctx, s.db, groupID); err != nil {
		return err
	}
	if _, err := s.UserByID(ctx, userID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO group_members (group_id, user_id) VALUES (?, ?)
		ON CONFLICT(group_id, user_id) DO NOTHING`, groupID, userID)
	return err
}

// RemoveGroupMember takes an account out of a group.
func (s *Store) RemoveGroupMember(ctx context.Context, groupID, userID int64) error {
	if err := s.changeable(ctx, s.db, groupID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, userID)
	return err
}

// SomaGroups lists the groups a soma is shared with, by role and name.
func (s *Store) SomaGroups(ctx context.Context, somaID string) ([]GroupGrant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+groupCols+`, sg.role
		FROM soma_groups sg JOIN groups g ON g.id = sg.group_id WHERE sg.soma_id = ?
		ORDER BY g.everyone DESC, `+rankSQL("sg.role")+` DESC, g.name COLLATE NOCASE`, somaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GroupGrant{}
	for rows.Next() {
		var x GroupGrant
		g, err := scanGroup(rows, &x.Role)
		if err != nil {
			return nil, err
		}
		x.Group = g
		out = append(out, x)
	}
	return out, rows.Err()
}

// SetSomaGroup shares a soma with a group, or changes the group's role;
// ErrNotFound for an unknown group.
func (s *Store) SetSomaGroup(ctx context.Context, somaID string, groupID int64, role string) error {
	if !IsShareRole(role) {
		return errors.New("invalid member role")
	}
	if _, err := s.GroupByID(ctx, groupID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO soma_groups (soma_id, group_id, role) VALUES (?, ?, ?)
		ON CONFLICT(soma_id, group_id) DO UPDATE SET role = excluded.role`, somaID, groupID, role)
	return err
}

// RemoveSomaGroup stops sharing a soma with a group.
func (s *Store) RemoveSomaGroup(ctx context.Context, somaID string, groupID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM soma_groups WHERE soma_id = ? AND group_id = ?`, somaID, groupID)
	return err
}

// GroupSomas lists the somas shared with a group (to tell their open clients
// when the group changes).
func (s *Store) GroupSomas(ctx context.Context, groupID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT soma_id FROM soma_groups WHERE group_id = ? ORDER BY soma_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
