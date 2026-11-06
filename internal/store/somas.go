package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"time"
)

// Member roles in a soma.
const (
	MemberOwner  = "owner"  // everything, including sharing and deleting
	MemberEditor = "editor" // read and write notes and attachments
	MemberViewer = "viewer" // read only
)

// RoleRank orders member roles: a role satisfies every role of lower rank.
func RoleRank(role string) int {
	switch role {
	case MemberOwner:
		return 3
	case MemberEditor:
		return 2
	case MemberViewer:
		return 1
	}
	return 0
}

// Soma is a registered soma: a folder of notes with an owner.
type Soma struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Folder    string    `json:"folder"` // relative to SOMAS_DIR, or absolute for an imported soma
	OwnerID   int64     `json:"ownerId"`
	CreatedAt time.Time `json:"createdAt"`
}

// Person is the public part of an account (shown to other users).
type Person struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

// Membership is a soma as one user sees it.
type Membership struct {
	Soma
	Role  string `json:"role"`
	Owner Person `json:"owner"`
}

// SomaInfo is a soma for the admin area.
type SomaInfo struct {
	Soma
	Owner   Person `json:"owner"`
	Members int    `json:"members"`
}

// Member is a person with access to a soma.
type Member struct {
	Person
	Role string `json:"role"`
}

// NewSomaID returns a random id for URLs (10 characters, a–z and 2–7).
func NewSomaID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	var b [10]byte
	_, _ = rand.Read(b[:])
	for i := range b {
		b[i] = alphabet[b[i]&31]
	}
	return string(b[:])
}

const somaCols = `s.id, s.name, s.folder, s.owner_id, s.created_at`

func scanSoma(row interface{ Scan(...any) error }, extra ...any) (Soma, error) {
	var (
		x       Soma
		created int64
	)
	err := row.Scan(append([]any{&x.ID, &x.Name, &x.Folder, &x.OwnerID, &created}, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return Soma{}, ErrNotFound
	}
	x.CreatedAt = fromUnix(created)
	return x, err
}

// CreateSoma registers a soma with its owner; ErrTaken if the folder is in use.
func (s *Store) CreateSoma(ctx context.Context, id, name, folder string, ownerID int64) (Soma, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Soma{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO somas (id, name, folder, owner_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, name, folder, ownerID, s.unixNow())
	if isUnique(err) {
		return Soma{}, ErrTaken
	}
	if err != nil {
		return Soma{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO soma_members (soma_id, user_id, role) VALUES (?, ?, 'owner')`, id, ownerID); err != nil {
		return Soma{}, err
	}
	if err := tx.Commit(); err != nil {
		return Soma{}, err
	}
	return s.SomaByID(ctx, id)
}

// SomaByID returns a registered soma.
func (s *Store) SomaByID(ctx context.Context, id string) (Soma, error) {
	return scanSoma(s.db.QueryRowContext(ctx, `SELECT `+somaCols+` FROM somas s WHERE s.id = ?`, id))
}

// CountSomas counts registered somas.
func (s *Store) CountSomas(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM somas`).Scan(&n)
	return n, err
}

// SomasOf lists the somas a user can open, by name.
func (s *Store) SomasOf(ctx context.Context, userID int64) ([]Membership, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+somaCols+`, m.role, o.id, o.username, o.display_name
		FROM soma_members m JOIN somas s ON s.id = m.soma_id JOIN users o ON o.id = s.owner_id
		WHERE m.user_id = ? ORDER BY s.name COLLATE NOCASE, s.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Membership{}
	for rows.Next() {
		var m Membership
		soma, err := scanSoma(rows, &m.Role, &m.Owner.ID, &m.Owner.Username, &m.Owner.DisplayName)
		if err != nil {
			return nil, err
		}
		m.Soma = soma
		out = append(out, m)
	}
	return out, rows.Err()
}

// AllSomas lists every soma (admin area).
func (s *Store) AllSomas(ctx context.Context) ([]SomaInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+somaCols+`, o.id, o.username, o.display_name,
		(SELECT COUNT(*) FROM soma_members m WHERE m.soma_id = s.id)
		FROM somas s JOIN users o ON o.id = s.owner_id ORDER BY s.name COLLATE NOCASE, s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SomaInfo{}
	for rows.Next() {
		var x SomaInfo
		soma, err := scanSoma(rows, &x.Owner.ID, &x.Owner.Username, &x.Owner.DisplayName, &x.Members)
		if err != nil {
			return nil, err
		}
		x.Soma = soma
		out = append(out, x)
	}
	return out, rows.Err()
}

// SomasOwnedBy lists the somas an account owns.
func (s *Store) SomasOwnedBy(ctx context.Context, userID int64) ([]Soma, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+somaCols+` FROM somas s WHERE s.owner_id = ? ORDER BY s.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Soma{}
	for rows.Next() {
		x, err := scanSoma(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// RenameSoma changes the display name (the folder stays).
func (s *Store) RenameSoma(ctx context.Context, id, name string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE somas SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSoma removes the registration (the caller takes care of the folder).
func (s *Store) DeleteSoma(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM somas WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// TransferSoma makes another account the owner; the previous owner stays as editor.
func (s *Store) TransferSoma(ctx context.Context, id string, newOwner int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old int64
	if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM somas WHERE id = ?`, id).Scan(&old); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if old == newOwner {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE somas SET owner_id = ? WHERE id = ?`, newOwner, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE soma_members SET role = 'editor' WHERE soma_id = ? AND user_id = ?`, id, old); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO soma_members (soma_id, user_id, role) VALUES (?, ?, 'owner')
		ON CONFLICT(soma_id, user_id) DO UPDATE SET role = 'owner'`, id, newOwner); err != nil {
		return err
	}
	return tx.Commit()
}

// MemberRole returns the role of a user in a soma ("" if none).
func (s *Store) MemberRole(ctx context.Context, somaID string, userID int64) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx, `SELECT role FROM soma_members WHERE soma_id = ? AND user_id = ?`, somaID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

// Members lists the people with access to a soma, owner first.
func (s *Store) Members(ctx context.Context, somaID string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.username, u.display_name, m.role
		FROM soma_members m JOIN users u ON u.id = m.user_id WHERE m.soma_id = ?
		ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'editor' THEN 1 ELSE 2 END, u.username COLLATE NOCASE`, somaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Username, &m.DisplayName, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetMember gives a user a role in a soma (not the owner role: see TransferSoma).
func (s *Store) SetMember(ctx context.Context, somaID string, userID int64, role string) error {
	if role != MemberEditor && role != MemberViewer {
		return errors.New("invalid member role")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO soma_members (soma_id, user_id, role) VALUES (?, ?, ?)
		ON CONFLICT(soma_id, user_id) DO UPDATE SET role = excluded.role WHERE soma_members.role != 'owner'`, somaID, userID, role)
	return err
}

// RemoveMember takes a user's access away (never the owner's).
func (s *Store) RemoveMember(ctx context.Context, somaID string, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM soma_members WHERE soma_id = ? AND user_id = ? AND role != 'owner'`, somaID, userID)
	return err
}

// --- Audit log ---------------------------------------------------------------

// AuditEntry is one recorded event.
type AuditEntry struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	ActorID int64     `json:"actorId"`
	Actor   string    `json:"actor"`
	Action  string    `json:"action"`
	Target  string    `json:"target"`
	Detail  string    `json:"detail"`
	IP      string    `json:"ip"`
}

// maxAuditEntries bounds the log; older entries are dropped.
const maxAuditEntries = 20000

// Audit records an event. Failures are logged, never returned: auditing must
// not break the action it records.
func (s *Store) Audit(ctx context.Context, e AuditEntry) {
	_, err := s.db.ExecContext(context.WithoutCancel(ctx), `INSERT INTO audit_log (at, actor_id, actor, action, target, detail, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, s.unixNow(), e.ActorID, e.Actor, e.Action, e.Target, e.Detail, e.IP)
	if err == nil {
		_, err = s.db.ExecContext(context.WithoutCancel(ctx), `DELETE FROM audit_log WHERE id <= (SELECT MAX(id) FROM audit_log) - ?`, maxAuditEntries)
	}
	if err != nil {
		s.log.Error("audit log write failed", "action", e.Action, "err", err)
	}
}

// AuditLog returns entries older than before (0: newest), newest first.
func (s *Store) AuditLog(ctx context.Context, before int64, limit int) ([]AuditEntry, error) {
	if before <= 0 {
		before = 1 << 62
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, at, actor_id, actor, action, target, detail, ip
		FROM audit_log WHERE id < ? ORDER BY id DESC LIMIT ?`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var (
			e  AuditEntry
			at int64
		)
		if err := rows.Scan(&e.ID, &at, &e.ActorID, &e.Actor, &e.Action, &e.Target, &e.Detail, &e.IP); err != nil {
			return nil, err
		}
		e.At = fromUnix(at)
		out = append(out, e)
	}
	return out, rows.Err()
}
