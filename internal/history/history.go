// Package history keeps earlier versions of the notes of a soma, in a
// SQLite database of its own (DATA_DIR/history/<soma id>.db). It is not a
// cache: it holds what exists nowhere else and belongs into backups. The soma
// itself stays free of extra files.
//
// Every write through dendronotes records the new content. Saves by the
// same people shortly after each other (an editing session) update the
// newest version instead of adding one, so the history shows sessions, not
// keystrokes.
package history

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"time"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
)

// Why a version was recorded.
const (
	ReasonEdit    = "edit"    // saved in dendronotes
	ReasonCreate  = "create"  // a new note
	ReasonOutside = "outside" // changed outside dendronotes
	ReasonRelink  = "relink"  // synapses rewritten after a rename
	ReasonRestore = "restore" // an earlier version restored
)

// Session is how long saves by the same people count as one version.
const Session = 10 * time.Minute

// ErrNotFound means there is no such version.
var ErrNotFound = errors.New("version not found")

const schemaVersion = 1

// Version describes a stored version (without its content).
type Version struct {
	ID      int64     `json:"id"`
	Path    string    `json:"path"`
	At      time.Time `json:"at"`
	Authors []int64   `json:"authors"`
	Reason  string    `json:"reason"`
	Size    int64     `json:"size"`
	// RestoredFrom is the version a restore brought back.
	RestoredFrom int64 `json:"restoredFrom,omitempty"`
}

// Store is the history of one soma.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// FileName returns the database file of a soma's history.
func FileName(dataDir, somaID string) string {
	return filepath.Join(dataDir, "history", somaID+".db")
}

// Open opens (or creates) a history database.
func Open(file string) (*Store, error) {
	db, err := driver.Open("file:"+filepath.ToSlash(file)+"?_txlock=immediate", func(c *sqlite3.Conn) error {
		return c.Exec(`PRAGMA busy_timeout = 10000;
			PRAGMA temp_store = MEMORY;
			PRAGMA synchronous = NORMAL;`)
	})
	if err != nil {
		return nil, fmt.Errorf("open history: %w", err)
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("open history: %w", err)
	}
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		db.Close()
		return nil, fmt.Errorf("open history: %w", err)
	}
	switch v {
	case 0:
		_, err = db.Exec(`CREATE TABLE versions (
			id            INTEGER PRIMARY KEY,
			path          TEXT NOT NULL,
			started       INTEGER NOT NULL,
			at            INTEGER NOT NULL,
			authors       TEXT NOT NULL DEFAULT '[]',
			reason        TEXT NOT NULL,
			restored_from INTEGER NOT NULL DEFAULT 0,
			size          INTEGER NOT NULL,
			hash          TEXT NOT NULL,
			content       BLOB NOT NULL
		);
		CREATE INDEX versions_path ON versions(path, at);
		PRAGMA user_version = 1;`)
	case schemaVersion:
	default:
		err = fmt.Errorf("history has schema version %d, this dendronotes knows only %d", v, schemaVersion)
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open history: %w", err)
	}
	return &Store{db: db, now: time.Now}, nil
}

// Close checkpoints and closes the database.
func (h *Store) Close() error {
	_, err := h.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return errors.Join(err, h.db.Close())
}

// Record stores content as the newest version of path. Unchanged content is
// skipped; an edit by the same people within Session of the newest version's
// start replaces it.
func (h *Store) Record(ctx context.Context, path string, content []byte, authors []int64, reason string) error {
	return h.record(ctx, path, content, authors, reason, 0)
}

// RecordRestore stores content brought back from version from.
func (h *Store) RecordRestore(ctx context.Context, path string, content []byte, authors []int64, from int64) error {
	return h.record(ctx, path, content, authors, ReasonRestore, from)
}

func (h *Store) record(ctx context.Context, path string, content []byte, authors []int64, reason string, from int64) error {
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	authors = slices.Sorted(slices.Values(authors))
	authors = slices.Compact(authors)
	if authors == nil {
		authors = []int64{}
	}
	who, _ := json.Marshal(authors)
	packed, err := pack(content)
	if err != nil {
		return err
	}
	now := h.now().UnixMilli()

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var (
		lastID                  int64
		lastStarted             int64
		lastHash, lastWho, last string
	)
	err = tx.QueryRowContext(ctx, `SELECT id, started, hash, authors, reason FROM versions WHERE path = ? ORDER BY at DESC, id DESC LIMIT 1`, path).
		Scan(&lastID, &lastStarted, &lastHash, &lastWho, &last)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	case lastHash == hash:
		return nil // nothing new
	case reason == ReasonEdit && last == ReasonEdit && lastWho == string(who) && now-lastStarted < Session.Milliseconds():
		_, err = tx.ExecContext(ctx, `UPDATE versions SET at = ?, size = ?, hash = ?, content = ? WHERE id = ?`,
			now, len(content), hash, packed, lastID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO versions (path, started, at, authors, reason, restored_from, size, hash, content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, path, now, now, string(who), reason, from, len(content), hash, packed)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func scanVersion(row interface{ Scan(...any) error }, extra ...any) (Version, error) {
	var v Version
	var at int64
	var who string
	err := row.Scan(append([]any{&v.ID, &v.Path, &at, &who, &v.Reason, &v.RestoredFrom, &v.Size}, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	v.At = time.UnixMilli(at).UTC()
	if err := json.Unmarshal([]byte(who), &v.Authors); err != nil {
		return v, err
	}
	return v, nil
}

const versionCols = `id, path, at, authors, reason, restored_from, size`

// List returns the versions of path, newest first.
func (h *Store) List(ctx context.Context, path string) ([]Version, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT `+versionCols+` FROM versions WHERE path = ? ORDER BY at DESC, id DESC`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Get returns a version with its content.
func (h *Store) Get(ctx context.Context, id int64) (Version, []byte, error) {
	var packed []byte
	v, err := scanVersion(h.db.QueryRowContext(ctx, `SELECT `+versionCols+`, content FROM versions WHERE id = ?`, id), &packed)
	if err != nil {
		return v, nil, err
	}
	content, err := unpack(packed)
	return v, content, err
}

// under matches path and everything below it (path as a folder).
const under = `(path = ?1 OR substr(path, 1, length(?1) + 1) = ?1 || '/')`

// Move follows a rename: the versions of from (and of everything below it,
// for a folder) now belong to to.
func (h *Store) Move(ctx context.Context, from, to string) error {
	if from == to {
		return nil
	}
	_, err := h.db.ExecContext(ctx, `UPDATE versions SET path = ?2 || substr(path, length(?1) + 1) WHERE `+under, from, to)
	return err
}

// Remove deletes the versions of path and of everything below it.
func (h *Store) Remove(ctx context.Context, path string) error {
	_, err := h.db.ExecContext(ctx, `DELETE FROM versions WHERE `+under, path)
	return err
}

// Prune deletes versions older than before, except the newest of each note.
func (h *Store) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := h.db.ExecContext(ctx, `DELETE FROM versions WHERE at < ? AND id NOT IN
		(SELECT id FROM (SELECT id, ROW_NUMBER() OVER (PARTITION BY path ORDER BY at DESC, id DESC) AS n FROM versions) WHERE n = 1)`,
		before.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Backup writes a consistent copy of the database to file (which must not exist).
func (h *Store) Backup(ctx context.Context, file string) error {
	_, err := h.db.ExecContext(ctx, `VACUUM INTO ?`, file)
	return err
}

func pack(content []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(content); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unpack(packed []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}
