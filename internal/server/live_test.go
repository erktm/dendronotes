package server

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		full := filepath.Join(cfg.SomaDir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := newTestServerWith(t, testFS(), cfg)
	if _, err := s.index.Sync(context.Background()); err != nil {
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
	expect(t, c.do("GET", "/api/soma/search?q=clay", ""), 401, "unauthorized")
	c.login()

	rec := c.do("GET", "/api/soma/search?q=clay", "")
	expect(t, rec, 200, "")
	res := decode[struct {
		Hits     []index.Hit `json:"hits"`
		Complete bool        `json:"complete"`
	}](t, rec)
	if len(res.Hits) != 1 || res.Hits[0].Path != "Soil types.md" || !res.Complete {
		t.Errorf("search: %+v", res)
	}
	expect(t, c.do("GET", "/api/soma/search?q=x&limit=0", ""), 400, "invalid_limit")
	expect(t, c.do("GET", "/api/soma/search?q="+strings.Repeat("a", 600), ""), 400, "query_too_long")

	rec = c.do("GET", "/api/soma/backlinks/Projects/Garden%20plan.md", "")
	expect(t, rec, 200, "")
	bl := decode[struct{ Backlinks []index.Backlink }](t, rec).Backlinks
	if len(bl) != 1 || bl[0].Path != "Journal/Log.md" || len(bl[0].Synapses) != 2 {
		t.Errorf("backlinks: %+v", bl)
	}
	expect(t, c.do("GET", "/api/soma/backlinks/..%2Fetc%2Fpasswd", ""), 400, "invalid_path")

	rec = c.do("GET", "/api/soma/tags", "")
	expect(t, rec, 200, "")
	if tags := decode[struct{ Tags []index.TagCount }](t, rec).Tags; len(tags) != 2 {
		t.Errorf("tags: %+v", tags)
	}
}

// subscribe returns the next change event published by the server.
func subscribe(t *testing.T, s *Server) func() index.Changes {
	t.Helper()
	ch, cancel, ok := s.hub.Subscribe()
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

	rec := c.do("GET", "/api/soma/notes/Soil%20types.md", "")
	etag := rec.Header().Get("ETag")
	rec = c.do("PUT", "/api/soma/notes/Soil%20types.md", `{"content":"Now peat. [[Journal/Log]]"}`, "If-Match", etag)
	expect(t, rec, 200, "")

	ch := next()
	if len(ch.Notes) != 1 || ch.Notes[0].Path != "Soil types.md" || `"`+ch.Notes[0].ETag+`"` != rec.Header().Get("ETag") {
		t.Errorf("change after save: %+v", ch)
	}
	hits, _ := s.index.Search(context.Background(), "peat", 10)
	if len(hits) != 1 {
		t.Errorf("search does not see the saved content: %+v", hits)
	}

	expect(t, c.do("POST", "/api/soma/folders/Archive", ""), 201, "")
	if ch := next(); !ch.Tree {
		t.Errorf("new folder not reported as tree change: %+v", ch)
	}
}

func TestMoveUpdatesSynapses(t *testing.T) {
	s := newIndexedServer(t, liveFiles)
	c := newClient(t, s)
	c.login()

	rec := c.do("POST", "/api/soma/move", `{"from":"Projects/Garden plan.md","to":"Projects/Plan 2026.md","updateSynapses":true}`)
	expect(t, rec, 200, "")
	res := decode[moveResult](t, rec)
	if res.Path != "Projects/Plan 2026.md" || res.Notes != 1 || res.Synapses != 2 || len(res.Failed) != 0 {
		t.Errorf("move result: %+v", res)
	}
	data, err := os.ReadFile(filepath.Join(s.cfg.SomaDir, "Journal", "Log.md"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Follow the [[Plan 2026]] and [plan](../Projects/Plan%202026.md)."; string(data) != want {
		t.Errorf("Log.md = %q, want %q", data, want)
	}
	if bl, _ := s.index.Backlinks(context.Background(), "Projects/Plan 2026.md"); len(bl) != 1 {
		t.Errorf("backlinks after move: %+v", bl)
	}

	// Without the option, nothing else is touched.
	rec = c.do("POST", "/api/soma/move", `{"from":"Soil types.md","to":"Soil.md"}`)
	expect(t, rec, 200, "")
	data, _ = os.ReadFile(filepath.Join(s.cfg.SomaDir, "Projects", "Plan 2026.md"))
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

	req, _ := http.NewRequest("GET", srv.URL+"/api/soma/events", nil)
	req.AddCookie(c.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
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

	s.publish(index.Changes{Notes: []index.NoteChange{{Path: "a.md", ETag: "x"}}})
	readUntil("event: change")
	if data := readUntil("data: "); data != `data: {"tree":false,"notes":[{"path":"a.md","etag":"x"}],"removed":[]}` {
		t.Errorf("change data: %s", data)
	}

	// Shutdown ends the stream.
	s.hub.Close()
	for lines.Scan() {
	}
	if s.hub.Subscribers() != 0 {
		t.Error("subscriber left behind")
	}
}

func TestEventStreamLimit(t *testing.T) {
	s := newIndexedServer(t, nil)
	s.hub = events.NewHub(0)
	c := newClient(t, s)
	c.login()
	expect(t, c.do("GET", "/api/soma/events", ""), 503, "too_many_streams")
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
	for !s.index.Ready() {
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
	full := filepath.Join(s.cfg.SomaDir, "Outside edit.md")
	if err := os.WriteFile(full, []byte("Written by [[another]] editor"), 0o644); err != nil {
		t.Fatal(err)
	}
	ch := next()
	if !ch.Tree || len(ch.Notes) != 1 || ch.Notes[0].Path != "Outside edit.md" {
		t.Errorf("change: %+v", ch)
	}
	hits, _ := s.index.Search(context.Background(), "editor", 10)
	if len(hits) != 1 {
		t.Errorf("search after external write: %+v", hits)
	}
}
