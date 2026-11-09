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
}

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
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate state to version %d: %w", v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
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
func isUnique(err error) bool {
	var e *sqlite3.Error
	return errors.As(err, &e) && e.ExtendedCode() == sqlite3.CONSTRAINT_UNIQUE
}
