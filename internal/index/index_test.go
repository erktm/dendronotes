package index

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/soma"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

type fixture struct {
	dir, data string
	soma      *soma.Soma
	ix        *Index
}

func newFixture(t *testing.T, files map[string]string) *fixture {
	t.Helper()
	f := &fixture{dir: t.TempDir(), data: t.TempDir()}
	for p, content := range files {
		f.write(t, p, content)
	}
	sm, err := soma.Open(f.dir, soma.Options{MaxNoteBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sm.Close() })
	f.soma = sm
	f.open(t)
	return f
}

func (f *fixture) open(t *testing.T) {
	t.Helper()
	ix, err := Open(f.data, f.soma, discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ix.Close() })
	f.ix = ix
}

func (f *fixture) write(t *testing.T, p, content string) {
	t.Helper()
	full := filepath.Join(f.dir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// A distinct modification time, also on file systems with coarse clocks.
	later := time.Now().Add(time.Duration(len(content)+1) * time.Second)
	if err := os.Chtimes(full, later, later); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) sync(t *testing.T) Changes {
	t.Helper()
	ch, err := f.ix.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

var garden = map[string]string{
	"Soil types.md":               "# Soil types\n\nLoam, clay and sand. #garden\n",
	"Projects/Garden plan.md":     "# Garden plan\n\nSee [[Soil types]] and [the log](../Journal/Garden%20log.md).\n\n- [ ] Order seeds #todo\n",
	"Journal/Garden log.md":       "Next spring, follow the [[Garden plan|plan]].\nAlso ![[Soil types]].\n",
	"Projects/Archive/Old.md":     "Old notes about [[Garden plan]] and [[Missing]].\n```\n[[Soil types]] in code\n```\n",
	"Café.md":                     "Crème brûlée recipe. #Kitchen/desserts\n",
	"photo.png":                   "not a note",
	".obsidian/workspace.md":      "[[Soil types]] hidden",
	"Projects/Twin/Soil types.md": "Another note with the same name.",
}

func TestSyncFindsNotesAndChanges(t *testing.T) {
	f := newFixture(t, garden)
	ch := f.sync(t)
	if !ch.Tree || len(ch.Notes) != 6 || len(ch.Removed) != 0 {
		t.Fatalf("first sync: %+v", ch)
	}
	if !f.ix.Ready() {
		t.Error("index not ready after the first sync")
	}
	if ch := f.sync(t); !ch.Empty() {
		t.Errorf("second sync without changes: %+v", ch)
	}

	f.write(t, "Soil types.md", "# Soil types\n\nChanged.\n")
	if err := os.Remove(filepath.Join(f.dir, "Café.md")); err != nil {
		t.Fatal(err)
	}
	ch = f.sync(t)
	if !ch.Tree || len(ch.Notes) != 1 || ch.Notes[0].Path != "Soil types.md" || !slices.Equal(ch.Removed, []string{"Café.md"}) {
		t.Errorf("after change and delete: %+v", ch)
	}
	if ch.Notes[0].ETag != soma.ETag([]byte("# Soil types\n\nChanged.\n")) {
		t.Errorf("etag %q does not match the content", ch.Notes[0].ETag)
	}
}

func TestSyncSkipsTouchedButUnchangedNotes(t *testing.T) {
	f := newFixture(t, garden)
	f.sync(t)
	f.write(t, "Soil types.md", garden["Soil types.md"]+"") // same content, new mtime
	later := time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(f.dir, "Soil types.md"), later, later)
	if ch := f.sync(t); !ch.Empty() {
		t.Errorf("touch reported as change: %+v", ch)
	}
}

func TestRefresh(t *testing.T) {
	f := newFixture(t, garden)
	f.sync(t)
	ctx := context.Background()

	f.write(t, "Journal/Garden log.md", "Rewritten without synapses.\n")
	ch, err := f.ix.Refresh(ctx, []string{"Journal/Garden log.md", "photo.png"})
	if err != nil {
		t.Fatal(err)
	}
	if ch.Tree || len(ch.Notes) != 1 || ch.Notes[0].Path != "Journal/Garden log.md" {
		t.Errorf("content refresh: %+v", ch)
	}
	if ch, _ := f.ix.Refresh(ctx, []string{"Journal/Garden log.md"}); !ch.Empty() {
		t.Errorf("repeated refresh: %+v", ch)
	}

	// Unknown paths fall back to a full sync.
	f.write(t, "New/Idea.md", "[[Soil types]]")
	ch, err = f.ix.Refresh(ctx, []string{"New/Idea.md"})
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Tree || len(ch.Notes) != 1 || ch.Notes[0].Path != "New/Idea.md" {
		t.Errorf("refresh of a new note: %+v", ch)
	}
}

func TestPut(t *testing.T) {
	f := newFixture(t, garden)
	f.sync(t)
	n, err := f.soma.ReadNote("Café.md")
	if err != nil {
		t.Fatal(err)
	}
	n, err = f.soma.WriteNote("Café.md", []byte("Now about [[Soil types]]."), n.ETag)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := f.ix.Put(context.Background(), n)
	if err != nil || len(ch.Notes) != 1 || ch.Notes[0].ETag != n.ETag {
		t.Fatalf("Put: %+v, %v", ch, err)
	}
	if ch := f.sync(t); !ch.Empty() {
		t.Errorf("sync after Put reports %+v", ch)
	}
	if bl := backlinkPaths(t, f.ix, "Soil types.md"); !slices.Contains(bl, "Café.md") {
		t.Errorf("backlinks after Put: %v", bl)
	}
}

func backlinkPaths(t *testing.T, ix *Index, target string) []string {
	t.Helper()
	bl, err := ix.Backlinks(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, b := range bl {
		out = append(out, b.Path)
	}
	return out
}

func TestBacklinks(t *testing.T) {
	f := newFixture(t, garden)
	f.sync(t)

	bl, err := f.ix.Backlinks(context.Background(), "Soil types.md")
	if err != nil {
		t.Fatal(err)
	}
	want := []Backlink{
		{Path: "Journal/Garden log.md", Synapses: []BacklinkEntry{{Line: 1, Context: "Also ![[Soil types]].", Embed: true}}},
		{Path: "Projects/Garden plan.md", Synapses: []BacklinkEntry{{Line: 2, Context: "See [[Soil types]] and [the log](../Journal/Garden%20log.md)."}}},
	}
	if !reflect.DeepEqual(bl, want) {
		t.Errorf("backlinks:\n got %+v\nwant %+v", bl, want)
	}

	// Same file name elsewhere: [[Soil types]] resolves to the top-level note,
	// so the twin has no backlinks; code and hidden folders never count.
	if got := backlinkPaths(t, f.ix, "Projects/Twin/Soil types.md"); len(got) != 0 {
		t.Errorf("twin backlinks: %v", got)
	}
	if got := backlinkPaths(t, f.ix, "Projects/Garden plan.md"); !slices.Equal(got, []string{"Journal/Garden log.md", "Projects/Archive/Old.md"}) {
		t.Errorf("garden plan backlinks: %v", got)
	}
	if got := backlinkPaths(t, f.ix, "Journal/Garden log.md"); !slices.Equal(got, []string{"Projects/Garden plan.md"}) {
		t.Errorf("Markdown link backlinks: %v", got)
	}
}

func TestSearch(t *testing.T) {
	f := newFixture(t, garden)
	f.sync(t)
	search := func(q string) []Hit {
		t.Helper()
		hits, err := f.ix.Search(context.Background(), q, 20)
		if err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		return hits
	}
	paths := func(hits []Hit) []string {
		var out []string
		for _, h := range hits {
			out = append(out, h.Path)
		}
		return out
	}

	if got := paths(search("clay")); !slices.Equal(got, []string{"Soil types.md"}) {
		t.Errorf("clay: %v", got)
	}
	hits := search("crem brul") // prefixes, without diacritics
	if len(hits) != 1 || hits[0].Path != "Café.md" || hits[0].Line != 0 {
		t.Fatalf("crem brul: %+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, MarkStart+"Crème"+MarkEnd) {
		t.Errorf("snippet without marks: %q", hits[0].Snippet)
	}
	if !slices.Equal(hits[0].Tags, []string{"Kitchen/desserts"}) {
		t.Errorf("tags of hit: %v", hits[0].Tags)
	}
	if got := search("garden plan"); len(got) == 0 || got[0].Path != "Projects/Garden plan.md" {
		t.Errorf("name match should rank first: %v", paths(got))
	}
	if got := paths(search(`"follow the"`)); !slices.Equal(got, []string{"Journal/Garden log.md"}) {
		t.Errorf("phrase: %v", got)
	}
	if got := paths(search("#kitchen")); !slices.Equal(got, []string{"Café.md"}) {
		t.Errorf("nested tag: %v", got)
	}
	if hits := search("#todo"); len(hits) != 1 || hits[0].Line != 4 || hits[0].Snippet != "- [ ] Order seeds "+MarkStart+"#todo"+MarkEnd {
		t.Errorf("tag search should point at the tag's line: %+v", hits)
	}
	if hits := search("garden plan"); hits[0].Line != 0 || len(search("archive")) != 1 || search("archive")[0].Snippet != "" || search("archive")[0].Line != -1 {
		t.Errorf("folder-only match should have no snippet: %+v", search("archive"))
	}
	if got := paths(search("#todo seeds")); !slices.Equal(got, []string{"Projects/Garden plan.md"}) {
		t.Errorf("tag and word: %v", got)
	}
	for _, q := range []string{"", "   ", `"`, "*", "#", "hidden"} {
		if got := search(q); len(got) != 0 {
			t.Errorf("Search(%q) = %v", q, paths(got))
		}
	}
	// FTS5 syntax in the input is quoted, never interpreted.
	for _, q := range []string{`AND OR NOT ("`, `title:clay`, `clay NEAR(sand)`, `"unclosed`, `-clay ^x`} {
		search(q)
	}
}

func TestTags(t *testing.T) {
	f := newFixture(t, map[string]string{
		"a.md": "#garden #todo",
		"b.md": "---\ntags: [Garden]\n---\n",
		"c.md": "#todo/later",
	})
	f.sync(t)
	tags, err := f.ix.Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []TagCount{{"Garden", 2}, {"todo", 1}, {"todo/later", 1}}
	if !reflect.DeepEqual(tags, want) {
		t.Errorf("tags = %+v, want %+v", tags, want)
	}
}

func TestReferrers(t *testing.T) {
	f := newFixture(t, garden)
	f.sync(t)
	got, err := f.ix.Referrers(context.Background(), []string{"Soil types.md"})
	if err != nil {
		t.Fatal(err)
	}
	// Candidates by name, including the code-free ones only (code is not indexed).
	if want := []string{"Journal/Garden log.md", "Projects/Garden plan.md"}; !slices.Equal(got, want) {
		t.Errorf("referrers = %v, want %v", got, want)
	}
}

func TestReopenKeepsIndex(t *testing.T) {
	f := newFixture(t, garden)
	f.sync(t)
	f.ix.Close()
	f.open(t)
	ch := f.sync(t)
	if len(ch.Notes) != 0 || len(ch.Removed) != 0 {
		t.Errorf("notes were indexed again after reopening: %+v", ch)
	}
}

func TestRebuildsOnSchemaMismatchAndDamage(t *testing.T) {
	f := newFixture(t, garden)
	f.sync(t)
	if _, err := f.ix.db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	f.ix.Close()
	f.open(t)
	if ch := f.sync(t); len(ch.Notes) != 6 {
		t.Errorf("schema mismatch: expected a rebuild, got %+v", ch)
	}

	f.ix.Close()
	if err := os.WriteFile(filepath.Join(f.data, FileName), []byte("this is not a database, just garbage bytes......"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	if ch := f.sync(t); len(ch.Notes) != 6 {
		t.Errorf("damaged file: expected a rebuild, got %+v", ch)
	}
}

func TestParseQuery(t *testing.T) {
	q := ParseQuery(`garden "north bed" #Todo/ x* -`)
	want := Query{Terms: []string{"garden", "x*"}, Phrases: []string{"north bed"}, Tags: []string{"todo"}}
	if !reflect.DeepEqual(q, want) {
		t.Errorf("ParseQuery = %+v, want %+v", q, want)
	}
	if got := q.ftsQuery(); got != `"garden"* "x*"* "north bed"` {
		t.Errorf("ftsQuery = %s", got)
	}
}
