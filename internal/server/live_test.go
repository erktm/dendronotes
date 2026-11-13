package server

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/events"
	"dendronotes/internal/index"
)

// newIndexedServer creates a server whose soma holds files, synced once.
func newIndexedServer(t *testing.T, files map[string]string) *Server {
	t.Helper()
	cfg := testConfig(t)
	for p, content := range files {
		full := filepath.Join(cfg.SomasDir, "test", filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := newTestServerWith(t, testFS(), cfg)
	if _, err := rtOf(t, s).Index.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

var liveFiles = map[string]string{
	"Soil types.md":           "Loam, clay and sand. #garden",
	"Projects/Garden plan.md": "See [[Soil types]].\n\n- [ ] Order seeds #todo",
	"Journal/Log.md":          "Follow the [[Garden plan]] and [plan](../Projects/Garden%20plan.md).",
}

func TestSearchBacklinksTagsAPI(t *testing.T) {
	s := newIndexedServer(t, liveFiles)
	c := newClient(t, s)
	expect(t, c.do("GET", "/api/somas/testsoma01/search?q=clay", ""), 401, "unauthorized")
	c.login()

	rec := c.do("GET", "/api/somas/testsoma01/search?q=clay", "")
	expect(t, rec, 200, "")
	res := decode[struct {
		Hits     []index.Hit `json:"hits"`
		Complete bool        `json:"complete"`
	}](t, rec)
	if len(res.Hits) != 1 || res.Hits[0].Path != "Soil types.md" || !res.Complete {
		t.Errorf("search: %+v", res)
	}
	expect(t, c.do("GET", "/api/somas/testsoma01/search?q=x&limit=0", ""), 400, "invalid_limit")
	expect(t, c.do("GET", "/api/somas/testsoma01/search?q="+strings.Repeat("a", 600), ""), 400, "query_too_long")

	rec = c.do("GET", "/api/somas/testsoma01/backlinks/Projects/Garden%20plan.md", "")
	expect(t, rec, 200, "")
	bl := decode[struct{ Backlinks []index.Backlink }](t, rec).Backlinks
	if len(bl) != 1 || bl[0].Path != "Journal/Log.md" || len(bl[0].Synapses) != 2 {
		t.Errorf("backlinks: %+v", bl)
	}
	expect(t, c.do("GET", "/api/somas/testsoma01/backlinks/..%2Fetc%2Fpasswd", ""), 400, "invalid_path")

	rec = c.do("GET", "/api/somas/testsoma01/graph", "")
	expect(t, rec, 200, "")
	if g := decode[index.Graph](t, rec); len(g.Nodes) != 3 || len(g.Edges) != 2 { // two synapses Log → plan are one edge
		t.Errorf("graph: %+v", g)
	}

	rec = c.do("GET", "/api/somas/testsoma01/tags", "")
	expect(t, rec, 200, "")
	if tags := decode[struct{ Tags []index.TagCount }](t, rec).Tags; len(tags) != 2 {
		t.Errorf("tags: %+v", tags)
	}
}

// Drawings are nodes of their own; the notes embedding or linking them are
// connected by embeds, never by synapses.
func TestGraphDrawings(t *testing.T) {
	s := newIndexedServer(t, map[string]string{
		"Projects/Garden plan.md":    "![[Beds.drawing.svg|400]] and ![plan](Beds.drawing.svg), [[Soil types]]",
		"Soil types.md":              "![[Projects/Beds.drawing.svg]] ![[photo.png]] ![[Gone.drawing.svg]]",
		"Projects/Beds.drawing.svg":  "<svg/>",
		"Archive/Unused.drawing.svg": "<svg/>",
		"photo.png":                  "png",
	})
	c := newClient(t, s)
	c.login()
	rec := c.do("GET", "/api/somas/testsoma01/graph", "")
	expect(t, rec, 200, "")
	g := decode[index.Graph](t, rec)
	var drawings []string
	for _, n := range g.Nodes {
		if n.Kind == "drawing" {
			drawings = append(drawings, n.ID)
		}
	}
	if !slices.Equal(drawings, []string{"Archive/Unused.drawing.svg", "Projects/Beds.drawing.svg"}) {
		t.Errorf("drawing nodes: %v", drawings)
	}
	want := []index.GraphEdge{
		{Source: "Projects/Garden plan.md", Target: "Projects/Beds.drawing.svg"},
		{Source: "Soil types.md", Target: "Projects/Beds.drawing.svg"},
	}
	if !slices.Equal(g.Embeds, want) {
		t.Errorf("embeds: %+v", g.Embeds)
	}
	if len(g.Edges) != 1 {
		t.Errorf("synapses: %+v", g.Edges)
	}
}

// subscribe returns the next change event published by the server.
func subscribe(t *testing.T, s *Server) func() index.Changes {
	t.Helper()
	_, ch, cancel, ok := rtOf(t, s).Hub.Subscribe()
	if !ok {
		t.Fatal("subscribe failed")
	}
	t.Cleanup(cancel)
	return func() index.Changes {
		t.Helper()
		select {
		case e := <-ch:
			if e.Name != "change" {
				t.Fatalf("event %q", e.Name)
			}
			return e.Data.(index.Changes)
		case <-time.After(10 * time.Second):
			t.Fatal("no change event")
			return index.Changes{}
		}
	}
}

func TestSaveUpdatesIndexAndPublishes(t *testing.T) {
	s := newIndexedServer(t, liveFiles)
	next := subscribe(t, s)
	c := newClient(t, s)
	c.login()

	rec := c.do("GET", "/api/somas/testsoma01/notes/Soil%20types.md", "")
	etag := rec.Header().Get("ETag")
	rec = c.do("PUT", "/api/somas/testsoma01/notes/Soil%20types.md", `{"content":"Now peat. [[Journal/Log]]"}`, "If-Match", etag)
	expect(t, rec, 200, "")

	ch := next()
	if len(ch.Notes) != 1 || ch.Notes[0].Path != "Soil types.md" || `"`+ch.Notes[0].ETag+`"` != rec.Header().Get("ETag") {
		t.Errorf("change after save: %+v", ch)
	}
	hits, _ := rtOf(t, s).Index.Search(context.Background(), "peat", 10)
	if len(hits) != 1 {
		t.Errorf("search does not see the saved content: %+v", hits)
	}

	expect(t, c.do("POST", "/api/somas/testsoma01/folders/Archive", ""), 201, "")
	if ch := next(); !ch.Tree {
		t.Errorf("new folder not reported as tree change: %+v", ch)
	}
}

func TestMoveUpdatesSynapses(t *testing.T) {
	s := newIndexedServer(t, liveFiles)
	c := newClient(t, s)
	c.login()

	rec := c.do("POST", "/api/somas/testsoma01/move", `{"from":"Projects/Garden plan.md","to":"Projects/Plan 2026.md","updateSynapses":true}`)
	expect(t, rec, 200, "")
	res := decode[moveResult](t, rec)
	if res.Path != "Projects/Plan 2026.md" || res.Notes != 1 || res.Synapses != 2 || len(res.Failed) != 0 {
		t.Errorf("move result: %+v", res)
	}
	data, err := os.ReadFile(filepath.Join(somaDir(s), "Journal", "Log.md"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Follow the [[Plan 2026]] and [plan](../Projects/Plan%202026.md)."; string(data) != want {
		t.Errorf("Log.md = %q, want %q", data, want)
	}
	if bl, _ := rtOf(t, s).Index.Backlinks(context.Background(), "Projects/Plan 2026.md"); len(bl) != 1 {
		t.Errorf("backlinks after move: %+v", bl)
	}

	// Without the option, nothing else is touched.
	rec = c.do("POST", "/api/somas/testsoma01/move", `{"from":"Soil types.md","to":"Soil.md"}`)
	expect(t, rec, 200, "")
	data, _ = os.ReadFile(filepath.Join(somaDir(s), "Projects", "Plan 2026.md"))
	if !strings.Contains(string(data), "[[Soil types]]") {
		t.Errorf("synapse rewritten without updateSynapses: %q", data)
	}
}

func TestEventStream(t *testing.T) {
	s := newIndexedServer(t, liveFiles)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	c := newClient(t, s)
	c.login()

	req, _ := http.NewRequest("GET", srv.URL+"/api/somas/testsoma01/events", nil)
	req.AddCookie(c.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// Proxies and CDNs must pass the stream through unchanged and at once.
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-transform") || resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Errorf("stream headers: Cache-Control %q, X-Accel-Buffering %q", cc, resp.Header.Get("X-Accel-Buffering"))
	}
	lines := bufio.NewScanner(resp.Body)
	readUntil := func(prefix string) string {
		t.Helper()
		for lines.Scan() {
			if strings.HasPrefix(lines.Text(), prefix) {
				return lines.Text()
			}
		}
		t.Fatalf("stream ended before %q: %v", prefix, lines.Err())
		return ""
	}
	readUntil("event: hello")

	rtOf(t, s).Publish(index.Changes{Notes: []index.NoteChange{{Path: "a.md", ETag: "x"}}})
	readUntil("event: change")
	if data := readUntil("data: "); data != `data: {"tree":false,"notes":[{"path":"a.md","etag":"x"}],"removed":[]}` {
		t.Errorf("change data: %s", data)
	}

	// Shutdown ends the stream.
	rtOf(t, s).Hub.Close()
	for lines.Scan() {
	}
	if rtOf(t, s).Hub.Subscribers() != 0 {
		t.Error("subscriber left behind")
	}
}

func TestEventStreamLimit(t *testing.T) {
	s := newIndexedServer(t, nil)
	rtOf(t, s).Hub = events.NewHub(0)
	c := newClient(t, s)
	c.login()
	expect(t, c.do("GET", "/api/somas/testsoma01/events", ""), 503, "too_many_streams")
}

// TestExternalChangesAreFollowed runs the real server: a file written by
// another program shows up in the index and as a change event.
func TestExternalChangesAreFollowed(t *testing.T) {
	s := newTestServer(t, testFS())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	}()

	deadline := time.Now().Add(10 * time.Second)
	for !rtOf(t, s).Index.Ready() {
		if time.Now().After(deadline) {
			t.Fatal("index not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	resp, err := http.Get("http://" + ln.Addr().String() + "/readyz")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("readyz: %v %v", resp, err)
	}
	resp.Body.Close()

	next := subscribe(t, s)
	full := filepath.Join(somaDir(s), "Outside edit.md")
	if err := os.WriteFile(full, []byte("Written by [[another]] editor"), 0o644); err != nil {
		t.Fatal(err)
	}
	ch := next()
	if !ch.Tree || len(ch.Notes) != 1 || ch.Notes[0].Path != "Outside edit.md" {
		t.Errorf("change: %+v", ch)
	}
	hits, _ := rtOf(t, s).Index.Search(context.Background(), "editor", 10)
	if len(hits) != 1 {
		t.Errorf("search after external write: %+v", hits)
	}
}

// Renaming or moving attachments rewrites the embeds and links to them.
func TestMoveUpdatesEmbeds(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	c.login()
	base := "/api/somas/" + testSoma
	expect(t, c.do("POST", base+"/notes/Garden/Beds.md", `{"content":"![[north.png|300]] and [map](north.png)"}`), 201, "")
	expect(t, c.upload(base+"/files/Garden/north.png", pngBytes), 201, "")
	expect(t, c.do("POST", base+"/notes/Log.md", `{"content":"![](Garden/north.png)"}`), 201, "")

	rec := c.do("POST", base+"/move", `{"from":"Garden/north.png","to":"Garden/north bed.png","updateSynapses":true}`)
	expect(t, rec, 200, "")
	if res := decode[moveResult](t, rec); res.Embeds != 3 || res.Notes != 2 || res.Synapses != 0 {
		t.Errorf("move result: %+v", res)
	}
	read := func(p string) string {
		data, _ := os.ReadFile(filepath.Join(somaDir(s), filepath.FromSlash(p)))
		return string(data)
	}
	if got := read("Garden/Beds.md"); got != "![[north bed.png|300]] and [map](north%20bed.png)" {
		t.Errorf("Beds.md = %q", got)
	}
	if got := read("Log.md"); got != "![](Garden/north%20bed.png)" {
		t.Errorf("Log.md = %q", got)
	}

	// A folder with an attachment moves: relative links follow, names still match.
	expect(t, c.do("POST", base+"/folders/Allotment", ``), 201, "")
	rec = c.do("POST", base+"/move", `{"from":"Garden","to":"Allotment/Garden","updateSynapses":true}`)
	expect(t, rec, 200, "")
	if got := read("Log.md"); got != "![](Allotment/Garden/north%20bed.png)" {
		t.Errorf("Log.md after the folder move = %q", got)
	}
	if got := read("Allotment/Garden/Beds.md"); got != "![[north bed.png|300]] and [map](north%20bed.png)" {
		t.Errorf("Beds.md after the folder move = %q", got)
	}
}
