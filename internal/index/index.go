// Package index keeps a searchable cache of the soma in SQLite: note
// versions, synapses (for backlinks), tags and a full-text index (FTS5).
//
// The soma is the source of truth. The database in DENDRONOTES_DATA_DIR can be
// deleted at any time; it is rebuilt on the next start, and also when its
// schema version does not match or the file is damaged.
package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"

	"dendronotes/internal/mdparse"
	"dendronotes/internal/soma"
)

// schemaVersion is stored as PRAGMA user_version; a different value makes
// Open discard the database and build a new one.
const schemaVersion = 3

// FileName is the database file inside the data directory.
const FileName = "index.db"

const schema = `
CREATE TABLE notes (
	id    INTEGER PRIMARY KEY,
	path  TEXT NOT NULL UNIQUE,
	size  INTEGER NOT NULL,
	mtime INTEGER NOT NULL,
	etag  TEXT NOT NULL
);
CREATE TABLE synapses (
	note_id  INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
	seq      INTEGER NOT NULL,
	name_key TEXT NOT NULL,
	target   TEXT NOT NULL,
	markdown INTEGER NOT NULL,
	embed    INTEGER NOT NULL,
	line     INTEGER NOT NULL,
	context  TEXT NOT NULL
);
CREATE INDEX synapses_name ON synapses(name_key);
CREATE INDEX synapses_note ON synapses(note_id);
CREATE TABLE file_refs (
	note_id  INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
	name_key TEXT NOT NULL,
	target   TEXT NOT NULL,
	markdown INTEGER NOT NULL
);
CREATE INDEX file_refs_name ON file_refs(name_key);
CREATE INDEX file_refs_note ON file_refs(note_id);
CREATE TABLE tags (
	note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
	tag     TEXT NOT NULL,
	fold    TEXT NOT NULL
);
CREATE INDEX tags_fold ON tags(fold);
CREATE INDEX tags_note ON tags(note_id);
CREATE VIRTUAL TABLE fts USING fts5(title, path, body, tokenize = 'unicode61 remove_diacritics 2');
`

// Index is the derived cache of one soma.
type Index struct {
	db   *sql.DB
	soma *soma.Soma
	log  *slog.Logger

	// mu serializes everything that writes: syncs, refreshes, puts.
	mu sync.Mutex
	// entries are all files and folders seen by the last sync (guarded by mu).
	entries map[string]soma.Entry

	resolver atomic.Pointer[mdparse.Resolver]
	ready    atomic.Bool
}

// Changes describes what a sync found, for live updates in clients.
type Changes struct {
	Tree    bool         `json:"tree"`    // files or folders appeared or disappeared
	Notes   []NoteChange `json:"notes"`   // notes with new content (including new notes)
	Removed []string     `json:"removed"` // notes that are gone
	// Moved are notes that went away and came back elsewhere with the same
	// content in one sync: moves made outside dendronotes (Obsidian, git).
	Moved []Move `json:"-"`
}

// Move is a note that moved from one path to another.
type Move struct {
	From, To string
}

// NoteChange is a note with its new version.
type NoteChange struct {
	Path string `json:"path"`
	ETag string `json:"etag"`
}

// Empty reports whether nothing changed.
func (c Changes) Empty() bool { return !c.Tree && len(c.Notes) == 0 && len(c.Removed) == 0 }

// Open opens or creates the index database in dataDir. A database with a
// different schema version, or one that cannot be read, is replaced.
func Open(dataDir string, sm *soma.Soma, log *slog.Logger) (*Index, error) {
	file := filepath.Join(dataDir, FileName)
	db, err := openDB(file)
	if err == nil {
		err = checkSchema(db)
		if err != nil {
			db.Close()
		}
	}
	if err != nil {
		log.Warn("index database unusable, rebuilding", "err", err)
		for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
			if rmErr := os.Remove(file + suffix); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				return nil, fmt.Errorf("remove index: %w", rmErr)
			}
		}
		if db, err = openDB(file); err != nil {
			return nil, err
		}
		if err = checkSchema(db); err != nil {
			db.Close()
			return nil, err
		}
	}
	ix := &Index{db: db, soma: sm, log: log, entries: map[string]soma.Entry{}}
	ix.resolver.Store(mdparse.NewResolver(nil))
	return ix, nil
}

func openDB(file string) (*sql.DB, error) {
	dsn := "file:" + filepath.ToSlash(file) + "?_txlock=immediate"
	db, err := driver.Open(dsn, func(c *sqlite3.Conn) error {
		if err := fts5.Register(c); err != nil {
			return err
		}
		// No temporary files: the container root file system is read-only.
		return c.Exec(`PRAGMA busy_timeout = 10000;
			PRAGMA foreign_keys = ON;
			PRAGMA temp_store = MEMORY;
			PRAGMA synchronous = NORMAL;`)
	})
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("open index: %w", err)
	}
	return db, nil
}

// checkSchema creates the schema in an empty database and verifies the
// version and integrity of an existing one.
func checkSchema(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	switch version {
	case 0:
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(schema); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
			return err
		}
		return tx.Commit()
	case schemaVersion:
		var result string
		if err := db.QueryRow(`PRAGMA quick_check`).Scan(&result); err != nil {
			return err
		}
		if result != "ok" {
			return fmt.Errorf("integrity check: %s", result)
		}
		return nil
	default:
		return fmt.Errorf("schema version %d, want %d", version, schemaVersion)
	}
}

// Close checkpoints the write-ahead log and closes the database.
func (ix *Index) Close() error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	_, err := ix.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return errors.Join(err, ix.db.Close())
}

// Ready reports whether the first complete sync has finished.
func (ix *Index) Ready() bool { return ix.ready.Load() }

// Resolver returns a resolver over all notes known to the index.
func (ix *Index) Resolver() *mdparse.Resolver { return ix.resolver.Load() }

type dbNote struct {
	id          int64
	size, mtime int64
	etag        string
}

// Sync reconciles the index with the soma: every file is compared by size
// and modification time, changed notes are read and indexed again.
func (ix *Index) Sync(ctx context.Context) (Changes, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.syncLocked(ctx)
}

func (ix *Index) syncLocked(ctx context.Context) (Changes, error) {
	tree, err := ix.soma.Tree()
	if err != nil {
		return Changes{}, err
	}
	var ch Changes
	entries := make(map[string]soma.Entry, len(tree))
	for _, e := range tree {
		entries[e.Path] = e
		if old, ok := ix.entries[e.Path]; !ok || old.Type != e.Type {
			ch.Tree = true
		}
	}
	if len(entries) != len(ix.entries) {
		ch.Tree = true
	}

	known, err := ix.loadNotes(ctx)
	if err != nil {
		return Changes{}, err
	}

	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return Changes{}, err
	}
	defer tx.Rollback()

	added := map[string][]string{} // new notes by content
	for _, e := range tree {
		if e.Type != "file" || !soma.IsNote(e.Path) {
			continue
		}
		old, exists := known[e.Path]
		delete(known, e.Path)
		if exists && old.size == e.Size && old.mtime == e.MTime {
			continue
		}
		if err := ctx.Err(); err != nil {
			return Changes{}, err
		}
		content, etag := ix.read(e.Path)
		if exists && old.etag == etag {
			if _, err := tx.Exec(`UPDATE notes SET size = ?, mtime = ? WHERE id = ?`, e.Size, e.MTime, old.id); err != nil {
				return Changes{}, err
			}
			continue
		}
		if err := putNote(tx, e.Path, e.Size, e.MTime, etag, content); err != nil {
			return Changes{}, err
		}
		ch.Notes = append(ch.Notes, NoteChange{Path: e.Path, ETag: etag})
		if !exists && etag != "" && etag != emptyETag {
			added[etag] = append(added[etag], e.Path)
		}
	}
	gone := map[string][]string{} // by content
	for p, old := range known {
		if err := deleteNote(tx, old.id); err != nil {
			return Changes{}, err
		}
		ch.Removed = append(ch.Removed, p)
		if old.etag != "" && old.etag != emptyETag {
			gone[old.etag] = append(gone[old.etag], p)
		}
	}
	ch.Moved = pairMoves(gone, added)
	if err := tx.Commit(); err != nil {
		return Changes{}, err
	}

	ix.entries = entries
	ix.storeResolver(entries)
	if !ix.ready.Load() {
		ix.ready.Store(true)
		ix.log.Info("index ready", "notes", len(ix.Resolver().Notes()))
	}
	return ch, nil
}

// emptyETag is the version of an empty note: empty notes are not told apart.
var emptyETag = soma.ETag(nil)

// pairMoves matches vanished and new notes by content. Only a content that
// vanished once and appeared once counts as a move; anything else stays a
// removal and a new note.
func pairMoves(gone, added map[string][]string) []Move {
	var out []Move
	for etag, from := range gone {
		to := added[etag]
		if len(from) == 1 && len(to) == 1 {
			out = append(out, Move{From: from[0], To: to[0]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

func (ix *Index) storeResolver(entries map[string]soma.Entry) {
	var notes []string
	for p, e := range entries {
		if e.Type == "file" && soma.IsNote(p) {
			notes = append(notes, p)
		}
	}
	ix.resolver.Store(mdparse.NewResolver(notes))
}

func (ix *Index) loadNotes(ctx context.Context) (map[string]dbNote, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT id, path, size, mtime, etag FROM notes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]dbNote{}
	for rows.Next() {
		var n dbNote
		var p string
		if err := rows.Scan(&n.id, &p, &n.size, &n.mtime, &n.etag); err != nil {
			return nil, err
		}
		out[p] = n
	}
	return out, rows.Err()
}

// read returns the content and version of a note. Notes that cannot be
// edited (too large, not UTF-8) are indexed by name only.
func (ix *Index) read(p string) (content, etag string) {
	n, err := ix.soma.ReadNote(p)
	if err != nil {
		if !errors.Is(err, soma.ErrTooLarge) && !errors.Is(err, soma.ErrNotNote) && !errors.Is(err, soma.ErrNotFound) {
			ix.log.Warn("index: cannot read note", "path", p, "err", err)
		}
		return "", ""
	}
	return n.Content, n.ETag
}

// Refresh re-checks the given paths after file system events. Paths that are
// known notes are compared and re-read where needed; anything else (new or
// vanished entries, folders) triggers a complete Sync.
func (ix *Index) Refresh(ctx context.Context, paths []string) (Changes, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	var ch Changes
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return Changes{}, err
	}
	defer tx.Rollback()
	for _, p := range paths {
		old, known := ix.entries[p]
		if !known || old.Type != "file" {
			tx.Rollback()
			return ix.syncLocked(ctx)
		}
		cur, err := ix.soma.Stat(p)
		if err != nil || cur.Type != "file" {
			tx.Rollback()
			return ix.syncLocked(ctx)
		}
		ix.entries[p] = cur
		if !soma.IsNote(p) || ix.hasVersion(ctx, p, cur) {
			continue
		}
		content, etag := ix.read(p)
		var dbEtag string
		err = tx.QueryRowContext(ctx, `SELECT etag FROM notes WHERE path = ?`, p).Scan(&dbEtag)
		if err == nil && dbEtag == etag {
			if _, err := tx.Exec(`UPDATE notes SET size = ?, mtime = ? WHERE path = ?`, cur.Size, cur.MTime, p); err != nil {
				return Changes{}, err
			}
			continue
		}
		if err := putNote(tx, p, cur.Size, cur.MTime, etag, content); err != nil {
			return Changes{}, err
		}
		ch.Notes = append(ch.Notes, NoteChange{Path: p, ETag: etag})
	}
	return ch, tx.Commit()
}

// hasVersion reports whether the database already holds the note with this
// size and modification time.
func (ix *Index) hasVersion(ctx context.Context, p string, e soma.Entry) bool {
	var size, mtime int64
	err := ix.db.QueryRowContext(ctx, `SELECT size, mtime FROM notes WHERE path = ?`, p).Scan(&size, &mtime)
	return err == nil && size == e.Size && mtime == e.MTime
}

// Put indexes a note dendronotes has just written, so that search and
// backlinks are current before the file system event arrives.
func (ix *Index) Put(ctx context.Context, n soma.Note) (Changes, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if old, ok := ix.entries[n.Path]; !ok || old.Type != "file" {
		return ix.syncLocked(ctx) // a new entry: let the sync find its folders too
	}
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return Changes{}, err
	}
	defer tx.Rollback()
	if err := putNote(tx, n.Path, n.Size, n.MTime, n.ETag, n.Content); err != nil {
		return Changes{}, err
	}
	if err := tx.Commit(); err != nil {
		return Changes{}, err
	}
	e := ix.entries[n.Path]
	e.Size, e.MTime = n.Size, n.MTime
	ix.entries[n.Path] = e
	return Changes{Notes: []NoteChange{{Path: n.Path, ETag: n.ETag}}}, nil
}

// putNote inserts or replaces a note with its synapses, tags and text.
func putNote(tx *sql.Tx, p string, size, mtime int64, etag, content string) error {
	var id int64
	err := tx.QueryRow(`INSERT INTO notes (path, size, mtime, etag) VALUES (?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET size = excluded.size, mtime = excluded.mtime, etag = excluded.etag
		RETURNING id`, p, size, mtime, etag).Scan(&id)
	if err != nil {
		return fmt.Errorf("index %s: %w", p, err)
	}
	for _, q := range []string{`DELETE FROM synapses WHERE note_id = ?`, `DELETE FROM file_refs WHERE note_id = ?`, `DELETE FROM tags WHERE note_id = ?`, `DELETE FROM fts WHERE rowid = ?`} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}

	doc := mdparse.Parse(content)
	lines := strings.Split(content, "\n")
	for i, s := range doc.Synapses {
		ctxLine := ""
		if s.Line < len(lines) {
			ctxLine = contextLine(lines[s.Line])
		}
		if _, err := tx.Exec(`INSERT INTO synapses (note_id, seq, name_key, target, markdown, embed, line, context)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, i, mdparse.NameKey(s.Target), s.Target, s.Markdown, s.Embed, s.Line, ctxLine); err != nil {
			return err
		}
	}
	for _, f := range doc.Files {
		if _, err := tx.Exec(`INSERT INTO file_refs (note_id, name_key, target, markdown) VALUES (?, ?, ?, ?)`,
			id, mdparse.Fold(path.Base(f.Target)), f.Target, f.Markdown); err != nil {
			return err
		}
	}
	for _, t := range doc.Tags {
		if _, err := tx.Exec(`INSERT INTO tags (note_id, tag, fold) VALUES (?, ?, ?)`, id, t, mdparse.Fold(t)); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`INSERT INTO fts (rowid, title, path, body) VALUES (?, ?, ?, ?)`, id, Title(p), mdparse.Dir(p), content)
	return err
}

func deleteNote(tx *sql.Tx, id int64) error {
	if _, err := tx.Exec(`DELETE FROM fts WHERE rowid = ?`, id); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM notes WHERE id = ?`, id)
	return err
}

// Title is the name shown for a note: its file name without ".md".
func Title(p string) string {
	name := path.Base(p)
	if strings.EqualFold(path.Ext(name), ".md") {
		return name[:len(name)-3]
	}
	return name
}

// contextLine shortens a source line for display next to a backlink.
func contextLine(line string) string {
	line = strings.TrimSpace(strings.TrimRight(line, "\r"))
	const max = 280
	if len(line) <= max {
		return line
	}
	cut := max
	for cut > 0 && !utf8RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + "…"
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }
