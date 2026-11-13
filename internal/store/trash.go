package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// TrashRecord remembers who moved an entry into a soma's .trash and from
// where. Path is relative to .trash.
type TrashRecord struct {
	SomaID    string
	Path      string
	Original  string
	Dir       bool
	DeletedBy *Person // nil when unknown or deleted since
	DeletedAt time.Time
}

// AddTrash records an entry moved to the trash.
func (s *Store) AddTrash(ctx context.Context, r TrashRecord) error {
	var by any
	if r.DeletedBy != nil && r.DeletedBy.ID != 0 {
		by = r.DeletedBy.ID
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO trash (soma_id, path, original, is_dir, deleted_by, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?)`, r.SomaID, r.Path, r.Original, r.Dir, by, s.unixNow())
	return err
}

// TrashRecords lists what is known about a soma's trash.
func (s *Store) TrashRecords(ctx context.Context, somaID string) ([]TrashRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.path, t.original, t.is_dir, t.deleted_at,
		COALESCE(u.id, 0), COALESCE(u.username, ''), COALESCE(u.display_name, '')
		FROM trash t LEFT JOIN users u ON u.id = t.deleted_by WHERE t.soma_id = ? ORDER BY t.deleted_at DESC, t.path`, somaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrashRecord
	for rows.Next() {
		r := TrashRecord{SomaID: somaID}
		var at int64
		var p Person
		if err := rows.Scan(&r.Path, &r.Original, &r.Dir, &at, &p.ID, &p.Username, &p.DisplayName); err != nil {
			return nil, err
		}
		r.DeletedAt = time.Unix(at, 0).UTC()
		if p.ID != 0 {
			r.DeletedBy = &p
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RemoveTrash forgets the record of path and of everything below it (after
// restoring or purging it).
func (s *Store) RemoveTrash(ctx context.Context, somaID, path string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM trash WHERE soma_id = ?1 AND (path = ?2 OR substr(path, 1, length(?2) + 1) = ?2 || '/')`,
		somaID, path)
	return err
}

// ClearTrash forgets every record of a soma's trash.
func (s *Store) ClearTrash(ctx context.Context, somaID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM trash WHERE soma_id = ?`, somaID)
	return err
}

// DeletedSoma is a soma in SOMAS_DIR/.trash that an admin can restore.
type DeletedSoma struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Folder         string    `json:"folder"` // its folder now: in the trash, or absolute (imported)
	OriginalFolder string    `json:"originalFolder"`
	Owner          *Person   `json:"owner"` // nil if the account is gone
	Members        int       `json:"members"`
	Groups         int       `json:"groups"`
	CreatedAt      time.Time `json:"createdAt"`
	DeletedBy      *Person   `json:"deletedBy"`
	DeletedAt      time.Time `json:"deletedAt"`

	memberRoles []roleOf
	groupRoles  []roleOf
}

// roleOf is a share kept with a deleted soma: an account or a group.
type roleOf struct {
	ID   int64  `json:"id"`
	Role string `json:"role"`
}

// DeleteSomaKeep unregisters a soma and keeps what is needed to restore it
// (name, owner, shares) with folder, where its files went.
func (s *Store) DeleteSomaKeep(ctx context.Context, id, folder string, by int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sm Soma
	var created int64
	err = tx.QueryRowContext(ctx, `SELECT id, name, folder, owner_id, created_at FROM somas WHERE id = ?`, id).
		Scan(&sm.ID, &sm.Name, &sm.Folder, &sm.OwnerID, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	members, err := collectRoles(ctx, tx, `SELECT user_id, role FROM soma_members WHERE soma_id = ? AND role <> 'owner'`, id)
	if err != nil {
		return err
	}
	groups, err := collectRoles(ctx, tx, `SELECT group_id, role FROM soma_groups WHERE soma_id = ?`, id)
	if err != nil {
		return err
	}
	var byID any
	if by != 0 {
		byID = by
	}
	_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO deleted_somas
		(id, name, folder, original_folder, owner_id, member_roles, group_roles, created_at, deleted_by, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sm.ID, sm.Name, folder, sm.Folder, sm.OwnerID, members, groups, created, byID, s.unixNow())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM somas WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func collectRoles(ctx context.Context, tx *sql.Tx, query, id string) (string, error) {
	rows, err := tx.QueryContext(ctx, query, id)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	list := []roleOf{}
	for rows.Next() {
		var r roleOf
		if err := rows.Scan(&r.ID, &r.Role); err != nil {
			return "", err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	b, err := json.Marshal(list)
	return string(b), err
}

const deletedCols = `d.id, d.name, d.folder, d.original_folder, d.member_roles, d.group_roles, d.created_at, d.deleted_at,
	COALESCE(o.id, 0), COALESCE(o.username, ''), COALESCE(o.display_name, ''),
	COALESCE(b.id, 0), COALESCE(b.username, ''), COALESCE(b.display_name, '')
	FROM deleted_somas d LEFT JOIN users o ON o.id = d.owner_id LEFT JOIN users b ON b.id = d.deleted_by`

func scanDeleted(row interface{ Scan(...any) error }) (DeletedSoma, error) {
	var d DeletedSoma
	var members, groups string
	var created, deleted int64
	var owner, by Person
	err := row.Scan(&d.ID, &d.Name, &d.Folder, &d.OriginalFolder, &members, &groups, &created, &deleted,
		&owner.ID, &owner.Username, &owner.DisplayName, &by.ID, &by.Username, &by.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal([]byte(members), &d.memberRoles); err != nil {
		return d, err
	}
	if err := json.Unmarshal([]byte(groups), &d.groupRoles); err != nil {
		return d, err
	}
	d.Members, d.Groups = len(d.memberRoles), len(d.groupRoles)
	d.CreatedAt, d.DeletedAt = time.Unix(created, 0).UTC(), time.Unix(deleted, 0).UTC()
	if owner.ID != 0 {
		d.Owner = &owner
	}
	if by.ID != 0 {
		d.DeletedBy = &by
	}
	return d, nil
}

// DeletedSomas lists the deleted somas, newest first.
func (s *Store) DeletedSomas(ctx context.Context) ([]DeletedSoma, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+deletedCols+` ORDER BY d.deleted_at DESC, d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeletedSoma{}
	for rows.Next() {
		d, err := scanDeleted(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeletedSomaByID returns one deleted soma.
func (s *Store) DeletedSomaByID(ctx context.Context, id string) (DeletedSoma, error) {
	return scanDeleted(s.db.QueryRowContext(ctx, `SELECT `+deletedCols+` WHERE d.id = ?`, id))
}

// RestoreSoma registers a deleted soma again with its files in folder and
// ownerID as owner. Its shares come back as far as their accounts and
// groups still exist.
func (s *Store) RestoreSoma(ctx context.Context, id, folder string, ownerID int64) (Soma, error) {
	d, err := s.DeletedSomaByID(ctx, id)
	if err != nil {
		return Soma{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Soma{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO somas (id, name, folder, owner_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		d.ID, d.Name, folder, ownerID, d.CreatedAt.Unix())
	if isUnique(err) {
		return Soma{}, ErrTaken
	}
	if err != nil {
		return Soma{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO soma_members (soma_id, user_id, role) VALUES (?, ?, 'owner')`, d.ID, ownerID); err != nil {
		return Soma{}, err
	}
	for _, m := range d.memberRoles {
		if m.ID == ownerID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO soma_members (soma_id, user_id, role)
			SELECT ?, id, ? FROM users WHERE id = ?`, d.ID, m.Role, m.ID); err != nil {
			return Soma{}, err
		}
	}
	for _, g := range d.groupRoles {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO soma_groups (soma_id, group_id, role)
			SELECT ?, id, ? FROM groups WHERE id = ?`, d.ID, g.Role, g.ID); err != nil {
			return Soma{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM deleted_somas WHERE id = ?`, d.ID); err != nil {
		return Soma{}, err
	}
	if err := tx.Commit(); err != nil {
		return Soma{}, err
	}
	return s.SomaByID(ctx, d.ID)
}

// ForgetDeletedSoma removes a deleted soma for good (with its trash records,
// comments, icons, sort orders and links).
func (s *Store) ForgetDeletedSoma(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM deleted_somas WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	for _, table := range []string{"trash", "notifications", "comments", "soma_icons", "icon_sections", "folder_order", "share_links"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE soma_id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
