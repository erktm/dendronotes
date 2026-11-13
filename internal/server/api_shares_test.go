package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/store"
)

func TestShareLinks(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	c.login()
	dir := somaDir(s)
	files := map[string]string{
		"Garden/Plan.md":   "# Plan\n\n![[bed.png]] and [[Secret]].\n",
		"Garden/bed.png":   "\x89PNG\r\n\x1a\n",
		"Garden/other.png": "\x89PNG\r\n\x1a\n",
		"Secret.md":        "not shared",
		"Map.drawing.svg":  `<svg xmlns="http://www.w3.org/2000/svg"/>`,
	}
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := "/api/somas/" + testSoma

	// Internal links: one per file, following it when it moves.
	l := decode[store.ShareLink](t, c.do("POST", base+"/links", `{"path":"Garden/Plan.md"}`))
	if again := decode[store.ShareLink](t, c.do("POST", base+"/links", `{"path":"Garden/Plan.md"}`)); again.ID != l.ID {
		t.Fatalf("a second internal link: %v vs %v", again.ID, l.ID)
	}
	expect(t, c.do("POST", base+"/links", `{"path":"Garden"}`), 400, "not_a_file")
	expect(t, c.do("POST", base+"/links", `{"path":"Gone.md"}`), 404, "not_found")
	expect(t, c.do("POST", base+"/move", `{"from":"Garden","to":"Beds"}`), 200, "")
	if got := decode[store.ShareLink](t, c.do("GET", base+"/links/"+l.ID, "")); got.Path != "Beds/Plan.md" {
		t.Fatalf("internal link after the move: %+v", got)
	}

	// Public links: only notes and drawings, the token only in the answer.
	expect(t, c.do("POST", base+"/shares", `{"path":"Beds/bed.png"}`), 400, "not_shareable")
	expect(t, c.do("POST", base+"/shares", `{"path":"Beds/Plan.md","days":400}`), 400, "invalid_days")
	type created struct {
		Token string          `json:"token"`
		Link  store.ShareLink `json:"link"`
	}
	pub := decode[created](t, c.do("POST", base+"/shares", `{"path":"Beds/Plan.md","days":7}`))
	if pub.Token == "" || pub.Link.ExpiresAt.IsZero() {
		t.Fatalf("public link: %+v", pub)
	}
	drawingLink := decode[created](t, c.do("POST", base+"/shares", `{"path":"Map.drawing.svg"}`))

	anon := newClient(t, s)
	type shown struct {
		Path    string   `json:"path"`
		Kind    string   `json:"kind"`
		Content string   `json:"content"`
		Files   []string `json:"files"`
		SVG     string   `json:"svg"`
	}
	rec := anon.do("GET", "/api/public/"+pub.Token, "")
	if rec.Header().Get("X-Robots-Tag") == "" {
		t.Error("a shared page may be indexed")
	}
	got := decode[shown](t, rec)
	if got.Kind != "note" || !strings.Contains(got.Content, "# Plan") || len(got.Files) != 1 || got.Files[0] != "Beds/bed.png" {
		t.Fatalf("shared note: %+v", got)
	}
	expect(t, anon.do("GET", "/api/public/"+pub.Token+"/files/Beds/bed.png", ""), 200, "")
	expect(t, anon.do("GET", "/api/public/"+pub.Token+"/files/Beds/other.png", ""), 404, "not_found")
	expect(t, anon.do("GET", "/api/public/"+pub.Token+"/files/Secret.md", ""), 404, "not_found")
	if d := decode[shown](t, anon.do("GET", "/api/public/"+drawingLink.Token, "")); d.Kind != "drawing" || !strings.Contains(d.SVG, "<svg") {
		t.Errorf("shared drawing: %+v", d)
	}
	expect(t, anon.do("GET", "/api/public/"+drawingLink.Token+"/files/Beds/bed.png", ""), 404, "not_found")
	expect(t, anon.do("GET", "/api/public/"+strings.Repeat("x", 43), ""), 404, "not_found")

	// In the trash the link shows nothing; restored, it works again.
	expect(t, c.do("DELETE", base+"/entries/Beds/Plan.md", ""), 200, "")
	expect(t, anon.do("GET", "/api/public/"+pub.Token, ""), 404, "not_found")
	expect(t, c.do("POST", base+"/trash/Beds/Plan.md", ""), 200, "")
	expect(t, anon.do("GET", "/api/public/"+pub.Token, ""), 200, "")

	// A move outside dendronotes is recognized by the content.
	if err := os.Rename(filepath.Join(dir, "Beds", "Plan.md"), filepath.Join(dir, "Plan moved.md")); err != nil {
		t.Fatal(err)
	}
	rt := rtOf(t, s)
	ch, err := rt.Index.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rt.Publish(ch)
	if got := decode[shown](t, anon.do("GET", "/api/public/"+pub.Token, "")); got.Path != "Plan moved.md" {
		t.Errorf("after a move outside: %+v", got)
	}
	if got := decode[store.ShareLink](t, c.do("GET", base+"/links/"+l.ID, "")); got.Path != "Plan moved.md" {
		t.Errorf("internal link after a move outside: %+v", got)
	}

	// Editors see and revoke public links; an expired one shows nothing.
	list := decode[struct {
		Links []store.ShareLink `json:"links"`
	}](t, c.do("GET", base+"/shares", ""))
	if len(list.Links) != 2 {
		t.Fatalf("public links: %+v", list.Links)
	}
	expect(t, c.do("DELETE", base+"/shares/"+l.ID, ""), 404, "not_found")
	expect(t, c.do("DELETE", base+"/shares/"+drawingLink.Link.ID, ""), 204, "")
	expect(t, anon.do("GET", "/api/public/"+drawingLink.Token, ""), 404, "not_found")
	old, _, err := s.store.CreatePublicLink(context.Background(), testSoma, "Plan moved.md", 0, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	expect(t, anon.do("GET", "/api/public/"+old, ""), 404, "link_expired")

	// API tokens never manage public links.
	for _, m := range []string{"GET", "POST", "DELETE"} {
		if tokenMay(route{method: m, pattern: "/api/somas/{soma}/shares", access: somaEditor}, m, store.ScopeWrite) {
			t.Errorf("a token may %s public links", m)
		}
	}
}
