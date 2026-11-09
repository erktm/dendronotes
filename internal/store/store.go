// Package store keeps the server's state in DATA_DIR/state.db: accounts,
// sessions, somas, memberships and the audit log.
//
// Unlike the search indexes this is not a cache: it must be backed up along
// with the somas. The schema evolves through numbered migrations; before an
// existing database is migrated, a copy is kept next to it.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
)

// FileName is the database file inside the data directory.
const FileName = "state.db"

// Errors returned by the store.
var (
	ErrNotFound = errors.New("not found")
	ErrTaken    = errors.New("already taken")
)

// migrations are applied in order; index i brings the schema to version i+1.
// Never change a released migration – append a new one.
var migrations = []string{
	// 1: accounts, sessions, somas, memberships, audit log
	`CREATE TABLE users (
		id            INTEGER PRIMARY KEY,
		username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
		display_name  TEXT NOT NULL DEFAULT '',
		email         TEXT NOT NULL DEFAULT '',
		role          TEXT NOT NULL CHECK (role IN ('admin', 'user')),
		source        TEXT NOT NULL CHECK (source IN ('local', 'oidc', 'proxy')),
		password_hash TEXT NOT NULL DEFAULT '',
		oidc_issuer   TEXT NOT NULL DEFAULT '',
		oidc_subject  TEXT NOT NULL DEFAULT '',
		disabled      INTEGER NOT NULL DEFAULT 0,
		created_at    INTEGER NOT NULL,
		last_login_at INTEGER NOT NULL DEFAULT 0
	);
	CREATE UNIQUE INDEX users_oidc ON users(oidc_issuer, oidc_subject) WHERE source = 'oidc';
	CREATE TABLE sessions (
		id         INTEGER PRIMARY KEY,
		token_hash TEXT NOT NULL UNIQUE,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL,
		seen_at    INTEGER NOT NULL,
		ip         TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX sessions_user ON sessions(user_id);
	CREATE TABLE somas (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		folder     TEXT NOT NULL UNIQUE,
		owner_id   INTEGER NOT NULL REFERENCES users(id),
		created_at INTEGER NOT NULL
	);
	CREATE TABLE soma_members (
		soma_id TEXT NOT NULL REFERENCES somas(id) ON DELETE CASCADE,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role    TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
		PRIMARY KEY (soma_id, user_id)
	);
	CREATE INDEX soma_members_user ON soma_members(user_id);
	CREATE TABLE audit_log (
		id       INTEGER PRIMARY KEY,
		at       INTEGER NOT NULL,
		actor_id INTEGER NOT NULL DEFAULT 0,
		actor    TEXT NOT NULL DEFAULT '',
		action   TEXT NOT NULL,
		target   TEXT NOT NULL DEFAULT '',
		detail   TEXT NOT NULL DEFAULT '',
		ip       TEXT NOT NULL DEFAULT ''
	);`,
	// 2: avatars – a designed picture per account and an optional photo
	`ALTER TABLE users ADD COLUMN avatar_style TEXT NOT NULL DEFAULT '';
	ALTER TABLE users ADD COLUMN avatar_photo INTEGER NOT NULL DEFAULT 0;
	CREATE TABLE avatars (
		user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		image   BLOB NOT NULL
	);`,
	// 3: finer roles (writer, contributor), groups with the built-in Everyone,
	// somas shared with groups; soma_access is every way to a soma.
	`CREATE TABLE groups (
		id         INTEGER PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
		everyone   INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	);
	CREATE UNIQUE INDEX groups_everyone ON groups(everyone) WHERE everyone = 1;
	INSERT INTO groups (name, everyone, created_at) VALUES ('Everyone', 1, CAST(strftime('%s', 'now') AS INTEGER));
	CREATE TABLE group_members (
		group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		PRIMARY KEY (group_id, user_id)
	);
	CREATE INDEX group_members_user ON group_members(user_id);
	CREATE TABLE soma_groups (
		soma_id  TEXT NOT NULL REFERENCES somas(id) ON DELETE CASCADE,
		group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		role     TEXT NOT NULL CHECK (role IN ('editor', 'contributor', 'writer', 'viewer')),
		PRIMARY KEY (soma_id, group_id)
	);
	CREATE INDEX soma_groups_group ON soma_groups(group_id);
	CREATE TABLE soma_members_v3 (
		soma_id TEXT NOT NULL REFERENCES somas(id) ON DELETE CASCADE,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role    TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'contributor', 'writer', 'viewer')),
		PRIMARY KEY (soma_id, user_id)
	);
	INSERT INTO soma_members_v3 (soma_id, user_id, role) SELECT soma_id, user_id, role FROM soma_members;
	DROP TABLE soma_members;
	ALTER TABLE soma_members_v3 RENAME TO soma_members;
	CREATE INDEX soma_members_user ON soma_members(user_id);
	CREATE VIEW soma_access (soma_id, user_id, role) AS
		SELECT soma_id, user_id, role FROM soma_members
		UNION ALL
		SELECT g.soma_id, m.user_id, g.role FROM soma_groups g JOIN group_members m ON m.group_id = g.group_id
		UNION ALL
		SELECT g.soma_id, u.id, g.role FROM soma_groups g JOIN groups e ON e.id = g.group_id AND e.everyone = 1
			CROSS JOIN users u WHERE u.disabled = 0;`,
	// 4: accounts and groups from Active Directory / LDAP. users is rebuilt
	// for the new source (a CHECK constraint cannot be changed), so this one
	// runs without foreign key enforcement (see rebuilds).
	`DROP VIEW soma_access;
	CREATE TABLE users_v4 (
		id            INTEGER PRIMARY KEY,
		username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
		display_name  TEXT NOT NULL DEFAULT '',
		email         TEXT NOT NULL DEFAULT '',
		role          TEXT NOT NULL CHECK (role IN ('admin', 'user')),
		source        TEXT NOT NULL CHECK (source IN ('local', 'oidc', 'proxy', 'ldap')),
		password_hash TEXT NOT NULL DEFAULT '',
		oidc_issuer   TEXT NOT NULL DEFAULT '',
		oidc_subject  TEXT NOT NULL DEFAULT '',
		ldap_id       TEXT NOT NULL DEFAULT '',
		disabled      INTEGER NOT NULL DEFAULT 0,
		created_at    INTEGER NOT NULL,
		last_login_at INTEGER NOT NULL DEFAULT 0,
		avatar_style  TEXT NOT NULL DEFAULT '',
		avatar_photo  INTEGER NOT NULL DEFAULT 0
	);
	INSERT INTO users_v4 (id, username, display_name, email, role, source, password_hash, oidc_issuer, oidc_subject,
		disabled, created_at, last_login_at, avatar_style, avatar_photo)
		SELECT id, username, display_name, email, role, source, password_hash, oidc_issuer, oidc_subject,
		disabled, created_at, last_login_at, avatar_style, avatar_photo FROM users;
	DROP TABLE users;
	ALTER TABLE users_v4 RENAME TO users;
	CREATE UNIQUE INDEX users_oidc ON users(oidc_issuer, oidc_subject) WHERE source = 'oidc';
	CREATE UNIQUE INDEX users_ldap ON users(ldap_id) WHERE source = 'ldap';
	ALTER TABLE groups ADD COLUMN source TEXT NOT NULL DEFAULT 'local' CHECK (source IN ('local', 'ldap'));
	ALTER TABLE groups ADD COLUMN ldap_id TEXT NOT NULL DEFAULT '';
	CREATE UNIQUE INDEX groups_ldap ON groups(ldap_id) WHERE source = 'ldap';
	CREATE VIEW soma_access (soma_id, user_id, role) AS
		SELECT soma_id, user_id, role FROM soma_members
		UNION ALL
		SELECT g.soma_id, m.user_id, g.role FROM soma_groups g JOIN group_members m ON m.group_id = g.group_id
		UNION ALL
		SELECT g.soma_id, u.id, g.role FROM soma_groups g JOIN groups e ON e.id = g.group_id AND e.everyone = 1
			CROSS JOIN users u WHERE u.disabled = 0;`,
	// 5: the trash – who deleted what from where (the files themselves are in
	// the soma's .trash) – and deleted somas, which admins can restore. Both
	// outlive their soma row on purpose (no foreign key on the soma).
	`CREATE TABLE trash (
		soma_id    TEXT NOT NULL,
		path       TEXT NOT NULL,
		original   TEXT NOT NULL,
		is_dir     INTEGER NOT NULL DEFAULT 0,
		deleted_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
		deleted_at INTEGER NOT NULL,
		PRIMARY KEY (soma_id, path)
	);
	CREATE TABLE deleted_somas (
		id              TEXT PRIMARY KEY,
		name            TEXT NOT NULL,
		folder          TEXT NOT NULL,
		original_folder TEXT NOT NULL,
		owner_id        INTEGER REFERENCES users(id) ON DELETE SET NULL,
		member_roles    TEXT NOT NULL DEFAULT '[]',
		group_roles     TEXT NOT NULL DEFAULT '[]',
		created_at      INTEGER NOT NULL,
		deleted_by      INTEGER REFERENCES users(id) ON DELETE SET NULL,
		deleted_at      INTEGER NOT NULL
	);`,
	// 6: comments on notes (threads anchored to a quote of the text) and
	// notifications of mentions and replies. Like the trash, they outlive
	// their soma row (a deleted soma can be restored).
	`CREATE TABLE comments (
		id          INTEGER PRIMARY KEY,
		soma_id     TEXT NOT NULL,
		path        TEXT NOT NULL,
		thread_id   INTEGER NOT NULL DEFAULT 0,
		author_id   INTEGER REFERENCES users(id) ON DELETE SET NULL,
		body        TEXT NOT NULL,
		quote       TEXT NOT NULL DEFAULT '',
		prefix      TEXT NOT NULL DEFAULT '',
		suffix      TEXT NOT NULL DEFAULT '',
		created_at  INTEGER NOT NULL,
		edited_at   INTEGER NOT NULL DEFAULT 0,
		resolved_at INTEGER NOT NULL DEFAULT 0,
		resolved_by INTEGER REFERENCES users(id) ON DELETE SET NULL
	);
	CREATE INDEX comments_path ON comments(soma_id, path);
	CREATE INDEX comments_thread ON comments(thread_id) WHERE thread_id <> 0;
	CREATE TABLE notifications (
		id         INTEGER PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		kind       TEXT NOT NULL CHECK (kind IN ('mention', 'reply')),
		soma_id    TEXT NOT NULL,
		path       TEXT NOT NULL,
		comment_id INTEGER NOT NULL,
		actor_id   INTEGER REFERENCES users(id) ON DELETE SET NULL,
		created_at INTEGER NOT NULL,
		read_at    INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX notifications_user ON notifications(user_id, created_at);`,
}

// rebuilds are the migrations (by index) that recreate a table others refer
// to. SQLite needs foreign key enforcement off for that, which cannot be
// switched inside a transaction: they run on a connection of their own, and
// the result is checked with foreign_key_check before it is committed.
var rebuilds = map[int]bool{3: true}

// Store is the state database.
type Store struct {
	db  *sql.DB
	log *slog.Logger
	now func() time.Time
}

// Open opens (or creates) DATA_DIR/state.db and migrates it.
func Open(dataDir string, log *slog.Logger) (*Store, error) {
	file := filepath.Join(dataDir, FileName)
	db, err := driver.Open("file:"+filepath.ToSlash(file)+"?_txlock=immediate", func(c *sqlite3.Conn) error {
		return c.Exec(`PRAGMA busy_timeout = 10000;
			PRAGMA foreign_keys = ON;
			PRAGMA temp_store = MEMORY;
			PRAGMA synchronous = FULL;`)
	})
	if err != nil {
		return nil, fmt.Errorf("open state: %w", err)
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("open state: %w", err)
	}
	s := &Store{db: db, log: log, now: time.Now}
	if err := s.migrate(file); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close checkpoints and closes the database.
func (s *Store) Close() error {
	_, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return errors.Join(err, s.db.Close())
}

// Backup writes a consistent copy of the database to file (which must not
// exist), while it stays in use.
func (s *Store) Backup(ctx context.Context, file string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, file)
	return err
}

// Ping reports whether the database answers (readiness).
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) migrate(file string) error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read state version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("state.db has schema version %d, this dendronotes knows only %d – use a newer release", version, len(migrations))
	}
	if version == len(migrations) {
		return nil
	}
	if version > 0 {
		backup := fmt.Sprintf("%s.v%d.bak", file, version)
		_ = os.Remove(backup)
		if _, err := s.db.Exec(`VACUUM INTO ?`, backup); err != nil {
			return fmt.Errorf("back up state before migrating: %w", err)
		}
		s.log.Info("state backed up before migration", "file", backup)
	}
	for v := version; v < len(migrations); v++ {
		if err := s.migrateTo(v); err != nil {
			return err
		}
	}
	if version > 0 {
		s.log.Info("state migrated", "from", version, "to", len(migrations))
	}
	return nil
}

func (s *Store) unixNow() int64 { return s.now().Unix() }

func fromUnix(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}

// isUnique reports whether err is a UNIQUE constraint violation.
// migrateTo applies migration v (to version v+1) in one transaction.
func (s *Store) migrateTo(v int) error {
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if rebuilds[v] {
		if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
			return err
		}
		defer conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
		return fmt.Errorf("migrate state to version %d: %w", v+1, err)
	}
	if rebuilds[v] {
		rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
		if err != nil {
			return err
		}
		broken := rows.Next()
		rows.Close()
		if broken {
			return fmt.Errorf("migrate state to version %d: foreign keys broken", v+1)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, v+1)); err != nil {
		return err
	}
	return tx.Commit()
}

func isUnique(err error) bool {
	var e *sqlite3.Error
	return errors.As(err, &e) && e.ExtendedCode() == sqlite3.CONSTRAINT_UNIQUE
}
