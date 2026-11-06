package index

import (
	"context"
	"path"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"dendronotes/internal/mdparse"
)

// Highlight markers in search snippets. Clients split on them and render the
// matches themselves, so no HTML ever travels from the index.
const (
	MarkStart = "\x02"
	MarkEnd   = "\x03"
)

// Hit is a search result.
type Hit struct {
	Path    string   `json:"path"`
	Snippet string   `json:"snippet"`
	Line    int      `json:"line"` // first line with a match, -1 if only the name matched
	Tags    []string `json:"tags,omitempty"`
}

// Query is a parsed search query.
type Query struct {
	Terms   []string // words, matched as prefixes
	Phrases []string // "quoted text", matched exactly
	Tags    []string // #tag filters (folded), nested tags included
}

// ParseQuery splits user input into words, "phrases" and #tags.
func ParseQuery(q string) Query {
	var out Query
	for q = strings.TrimSpace(q); q != ""; q = strings.TrimSpace(q) {
		if q[0] == '"' {
			end := strings.IndexByte(q[1:], '"')
			phrase := q[1:]
			if end >= 0 {
				phrase, q = q[1:end+1], q[end+2:]
			} else {
				q = ""
			}
			if hasWordChar(phrase) {
				out.Phrases = append(out.Phrases, strings.TrimSpace(phrase))
			}
			continue
		}
		word, rest, _ := strings.Cut(q, " ")
		q = rest
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			out.Tags = append(out.Tags, mdparse.Fold(strings.Trim(word[1:], "/")))
		} else if hasWordChar(word) {
			out.Terms = append(out.Terms, word)
		}
	}
	return out
}

func hasWordChar(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) >= 0
}

// ftsQuery builds an FTS5 expression: every word as a quoted prefix query,
// every phrase quoted; all must match. Quoting neutralizes FTS5 syntax.
func (q Query) ftsQuery() string {
	var parts []string
	for _, t := range q.Terms {
		parts = append(parts, `"`+strings.ReplaceAll(t, `"`, `""`)+`"*`)
	}
	for _, p := range q.Phrases {
		parts = append(parts, `"`+strings.ReplaceAll(p, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " ")
}

// Search finds notes matching q, best matches first.
func (ix *Index) Search(ctx context.Context, input string, limit int) ([]Hit, error) {
	q := ParseQuery(input)
	if len(q.Terms) == 0 && len(q.Phrases) == 0 && len(q.Tags) == 0 {
		return []Hit{}, nil
	}

	var (
		sb   strings.Builder
		args []any
	)
	if fq := q.ftsQuery(); fq != "" {
		sb.WriteString(`SELECT n.id, n.path, snippet(fts, 2, char(2), char(3), '…', 12), fts.body
			FROM fts JOIN notes n ON n.id = fts.rowid WHERE fts MATCH ?`)
		args = append(args, fq)
	} else {
		sb.WriteString(`SELECT n.id, n.path, substr(fts.body, 1, 160), fts.body
			FROM fts JOIN notes n ON n.id = fts.rowid WHERE 1`)
	}
	for _, t := range q.Tags {
		sb.WriteString(` AND n.id IN (SELECT note_id FROM tags WHERE fold = ? OR fold LIKE ? ESCAPE '\')`)
		args = append(args, t, escapeLike(t)+"/%")
	}
	if len(q.Terms)+len(q.Phrases) > 0 {
		// Name and folder weigh more than the text.
		sb.WriteString(` ORDER BY bm25(fts, 8.0, 3.0, 1.0)`)
	} else {
		sb.WriteString(` ORDER BY n.path`)
	}
	sb.WriteString(` LIMIT ?`)
	args = append(args, limit)

	rows, err := ix.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := []Hit{}
	var ids []int64
	for rows.Next() {
		var (
			id   int64
			h    Hit
			body string
		)
		if err := rows.Scan(&id, &h.Path, &h.Snippet, &body); err != nil {
			return nil, err
		}
		h.Snippet = strings.Join(strings.Fields(h.Snippet), " ")
		if len(q.Terms)+len(q.Phrases) > 0 {
			h.Line = firstMatchLine(body, q)
			if h.Line < 0 {
				h.Snippet = "" // only the name or folder matched
			}
		} else if line, text := tagLine(body, q.Tags); line >= 0 {
			h.Line, h.Snippet = line, text
		} else {
			h.Line = -1 // tag from the front matter: keep the start of the note
		}
		hits = append(hits, h)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, id := range ids {
		if hits[i].Tags, err = ix.noteTags(ctx, id); err != nil {
			return nil, err
		}
	}
	return hits, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// firstMatchLine returns the first line containing a word or phrase of q,
// compared without case and diacritics like the full-text index, or -1.
func firstMatchLine(body string, q Query) int {
	var needles []string
	for _, t := range append(append([]string{}, q.Terms...), q.Phrases...) {
		needles = append(needles, plain(t))
	}
	if len(needles) == 0 {
		return -1
	}
	for i, line := range strings.Split(body, "\n") {
		l := plain(line)
		for _, n := range needles {
			if strings.Contains(l, n) {
				return i
			}
		}
	}
	return -1
}

// tagLine finds the first line with one of the tags (or a nested tag below
// it) and returns it with the tag marked.
func tagLine(body string, tags []string) (int, string) {
	for i, line := range strings.Split(body, "\n") {
		lower := strings.ToLower(line)
		for _, t := range tags {
			j := strings.Index(lower, "#"+t)
			if j < 0 {
				continue
			}
			end := j + 1 + len(t)
			if end < len(lower) && !strings.ContainsRune(" \t\r/.,;:!?)", rune(lower[end])) {
				continue // a longer tag such as #gardening
			}
			text := strings.TrimSpace(line)
			if len(lower) == len(line) { // offsets carry over: mark the tag
				lead := len(line) - len(strings.TrimLeft(line, " \t"))
				text = strings.TrimRight(line[lead:j]+MarkStart+line[j:end]+MarkEnd+line[end:], " \t\r")
			}
			return i, contextLine(text)
		}
	}
	return -1, ""
}

// plain lowercases s and strips diacritics.
func plain(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	return strings.ToLower(out)
}

func (ix *Index) noteTags(ctx context.Context, id int64) ([]string, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT tag FROM tags WHERE note_id = ? ORDER BY rowid`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// Backlink is a note with synapses to another note.
type Backlink struct {
	Path     string          `json:"path"`
	Synapses []BacklinkEntry `json:"synapses"`
}

// BacklinkEntry is one synapse in the referring note.
type BacklinkEntry struct {
	Line    int    `json:"line"`
	Context string `json:"context"`
	Embed   bool   `json:"embed,omitempty"`
}

// Backlinks lists the notes with synapses that resolve to target ("Incoming
// synapses"), in path order. Synapses of a note to itself are left out.
func (ix *Index) Backlinks(ctx context.Context, target string) ([]Backlink, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT n.path, s.target, s.markdown, s.embed, s.line, s.context
		FROM synapses s JOIN notes n ON n.id = s.note_id
		WHERE s.name_key = ? ORDER BY n.path, s.seq`, mdparse.Fold(path.Base(target)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := ix.Resolver()
	out := []Backlink{}
	for rows.Next() {
		var (
			src, t          string
			markdown, embed bool
			e               BacklinkEntry
		)
		if err := rows.Scan(&src, &t, &markdown, &embed, &e.Line, &e.Context); err != nil {
			return nil, err
		}
		if src == target || res.Resolve(t, markdown, src) != target {
			continue
		}
		e.Embed = embed
		if n := len(out); n > 0 && out[n-1].Path == src {
			out[n-1].Synapses = append(out[n-1].Synapses, e)
		} else {
			out = append(out, Backlink{Path: src, Synapses: []BacklinkEntry{e}})
		}
	}
	return out, rows.Err()
}

// Referrers returns the notes that may contain synapses to any of the given
// notes (same file name); callers resolve each synapse to be sure.
func (ix *Index) Referrers(ctx context.Context, targets []string) ([]string, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	keys := make([]any, 0, len(targets))
	for _, t := range targets {
		keys = append(keys, mdparse.Fold(path.Base(t)))
	}
	q := `SELECT DISTINCT n.path FROM synapses s JOIN notes n ON n.id = s.note_id
		WHERE s.name_key IN (?` + strings.Repeat(", ?", len(keys)-1) + `) ORDER BY n.path`
	rows, err := ix.db.QueryContext(ctx, q, keys...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// TagCount is a tag with the number of notes carrying it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Tags lists all tags, most used first.
func (ix *Index) Tags(ctx context.Context) ([]TagCount, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT MIN(tag), COUNT(DISTINCT note_id) AS c FROM tags
		GROUP BY fold ORDER BY c DESC, fold`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TagCount{}
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
