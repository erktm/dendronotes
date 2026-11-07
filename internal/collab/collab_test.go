package collab

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dendronotes/internal/events"
	"dendronotes/internal/soma"
)

// sink records what the manager sends to which stream.
type sink struct {
	mu   sync.Mutex
	evts map[string][]events.Event
}

func (s *sink) send(ids []string, e events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		s.evts[id] = append(s.evts[id], e)
	}
}

// take returns and forgets the events of a stream with the given name.
func (s *sink) take(id, name string) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	rest := s.evts[id][:0]
	for _, e := range s.evts[id] {
		if e.Name != name {
			rest = append(rest, e)
			continue
		}
		b, _ := json.Marshal(e.Data)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		out = append(out, m)
	}
	s.evts[id] = rest
	return out
}

type fixture struct {
	t     *testing.T
	dir   string
	soma  *soma.Soma
	m     *Manager
	sink  *sink
	saved chan string
}

func newFixture(t *testing.T, files map[string]string) *fixture {
	t.Helper()
	dir := t.TempDir()
	for p, c := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sm, err := soma.Open(dir, soma.Options{MaxNoteBytes: 1 << 16})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, dir: dir, soma: sm, sink: &sink{evts: map[string][]events.Event{}}, saved: make(chan string, 100)}
	f.m = New(Options{
		Soma: sm, MaxBytes: 1 << 16, Send: f.sink.send, Log: slog.New(slog.DiscardHandler),
		Saved:     func(n soma.Note) { f.saved <- n.Path },
		SaveDelay: 20 * time.Millisecond, MaxDelay: 100 * time.Millisecond, Linger: 30 * time.Millisecond,
	})
	t.Cleanup(func() { f.m.Close(); sm.Close() })
	return f
}

func (f *fixture) file(p string) string {
	b, err := os.ReadFile(filepath.Join(f.dir, p))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

// waitSaved waits until p is saved (earlier saves of other notes are skipped).
func (f *fixture) waitSaved(p string) {
	f.t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case got := <-f.saved:
			if got == p {
				return
			}
		case <-timeout:
			f.t.Fatalf("%s was not saved", p)
		}
	}
}

// edit builds an update replacing [from, to) with text, as CodeMirror would.
func edit(docLen, from, to int, text string) Update {
	var parts []any
	if from > 0 {
		parts = append(parts, from)
	}
	r := []any{to - from}
	for _, l := range strings.Split(text, "\n") {
		if text != "" {
			r = append(r, l)
		}
	}
	parts = append(parts, r)
	if docLen > to {
		parts = append(parts, docLen-to)
	}
	b, _ := json.Marshal(parts)
	return Update{ClientID: "c", Changes: b}
}

func TestJoinPushPullAndSave(t *testing.T) {
	f := newFixture(t, map[string]string{"Garden.md": "\ufeffBeds\r\nSoil\r\n"})
	f.m.AddStream("s1", User{ID: 1, Name: "Erik"})
	f.m.AddStream("s2", User{ID: 2, Name: "Anna"})

	a, err := f.m.Join("s1", 1, "Garden.md", nil)
	if err != nil || a.Text != "Beds\nSoil\n" || a.Version != 0 || len(a.Peers) != 1 {
		t.Fatalf("join: %+v %v", a, err)
	}
	b, err := f.m.Join("s2", 2, "Garden.md", nil)
	if err != nil || b.Doc != a.Doc || len(b.Peers) != 2 {
		t.Fatalf("second join: %+v %v", b, err)
	}
	if _, err := f.m.Join("s1", 2, "Garden.md", nil); !errors.Is(err, ErrUnknownStream) {
		t.Errorf("someone else's stream: %v", err)
	}

	// Erik types; both tabs receive the update in order.
	v, err := f.m.Push("s1", 1, "Garden.md", a.Doc, 0, []Update{edit(10, 4, 4, " and paths")}, false)
	if err != nil || v != 1 {
		t.Fatalf("push: %d %v", v, err)
	}
	for _, s := range []string{"s1", "s2"} {
		if ev := f.sink.take(s, "collab"); len(ev) != 1 || ev[0]["from"] != 0.0 {
			t.Errorf("%s got %v", s, ev)
		}
	}
	// Anna was not up to date: refused, catches up, retries.
	if v, err := f.m.Push("s2", 2, "Garden.md", a.Doc, 0, []Update{edit(10, 0, 0, "# ")}, false); !errors.Is(err, ErrVersion) || v != 1 {
		t.Errorf("stale push: %d %v", v, err)
	}
	ups, v, err := f.m.Pull("Garden.md", a.Doc, 0)
	if err != nil || len(ups) != 1 || v != 1 {
		t.Fatalf("pull: %v %d %v", ups, v, err)
	}
	if _, err := f.m.Push("s2", 2, "Garden.md", a.Doc, 1, []Update{edit(20, 0, 0, "# ")}, false); err != nil {
		t.Fatal(err)
	}

	// Saved shortly after, with the file's byte order mark and line breaks.
	f.waitSaved("Garden.md")
	if got := f.file("Garden.md"); got != "\ufeff# Beds and paths\r\nSoil\r\n" {
		t.Errorf("file: %q", got)
	}
	if ev := f.sink.take("s1", "saved"); len(ev) != 1 || ev[0]["version"] != 2.0 {
		t.Errorf("saved events: %v", ev)
	}

	// Broken or oversized updates change nothing.
	for _, u := range []Update{
		{ClientID: "c", Changes: json.RawMessage(`[3]`)},
		{ClientID: "c", Changes: json.RawMessage(`"x"`)},
		{ClientID: strings.Repeat("c", 100), Changes: json.RawMessage(`[22]`)},
	} {
		if _, err := f.m.Push("s1", 1, "Garden.md", a.Doc, 2, []Update{u}, false); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", u.Changes, err)
		}
	}
	big := edit(22, 22, 22, strings.Repeat("x", 1<<16))
	if _, err := f.m.Push("s1", 1, "Garden.md", a.Doc, 2, []Update{big}, false); !errors.Is(err, soma.ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}

	// Presence reaches the others with clamped positions.
	if err := f.m.Presence("s1", 1, "Garden.md", a.Doc, &Selection{Anchor: 3, Head: 999}); err != nil {
		t.Fatal(err)
	}
	ev := f.sink.take("s2", "presence")
	last := ev[len(ev)-1]["peers"].([]any)
	found := false
	for _, p := range last {
		if p := p.(map[string]any); p["name"] == "Erik" && p["sel"] != nil {
			found = p["sel"].(map[string]any)["head"] == 22.0
		}
	}
	if !found {
		t.Errorf("presence: %v", last)
	}

	// The document outlives a short absence, then closes; a new instance
	// starts later.
	f.m.RemoveStream("s1")
	f.m.RemoveStream("s2")
	if _, _, err := f.m.Pull("Garden.md", a.Doc, 0); err != nil {
		t.Errorf("pull right after leaving: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if _, _, err := f.m.Pull("Garden.md", a.Doc, 0); !errors.Is(err, ErrResync) {
		t.Errorf("pull after close: %v", err)
	}
	f.m.AddStream("s3", User{ID: 1, Name: "Erik"})
	c, err := f.m.Join("s3", 1, "Garden.md", nil)
	if err != nil || c.Doc == a.Doc || c.Text != "# Beds and paths\nSoil\n" {
		t.Errorf("rejoin: %+v %v", c, err)
	}
}

func TestChangesOnDiskAreMerged(t *testing.T) {
	f := newFixture(t, map[string]string{"Plan.md": "one\ntwo\nthree\n"})
	f.m.AddStream("s1", User{ID: 1, Name: "Erik"})
	j, _ := f.m.Join("s1", 1, "Plan.md", nil)
	if _, err := f.m.Push("s1", 1, "Plan.md", j.Doc, 0, []Update{edit(14, 0, 3, "ONE")}, false); err != nil {
		t.Fatal(err)
	}
	// Someone edits the file elsewhere before the save.
	if err := os.WriteFile(filepath.Join(f.dir, "Plan.md"), []byte("one\ntwo\nTHREE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.m.FileChanged("Plan.md")
	f.waitSaved("Plan.md")
	if got := f.file("Plan.md"); got != "ONE\ntwo\nTHREE\n" {
		t.Errorf("merged file: %q", got)
	}
	if n := f.sink.take("s1", "notice"); len(n) != 1 || n[0]["kind"] != "merged" {
		t.Errorf("notice: %v", n)
	}
	// Without local edits a change on disk simply arrives.
	if err := os.WriteFile(filepath.Join(f.dir, "Plan.md"), []byte("ONE\ntwo\nTHREE\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.m.FileChanged("Plan.md")
	deadline := time.Now().Add(2 * time.Second)
	for {
		ups, _, _ := f.m.Pull("Plan.md", j.Doc, 0)
		if len(ups) >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("change on disk not applied")
		}
		time.Sleep(10 * time.Millisecond)
	}
	j2, _ := f.m.Join("s1", 1, "Plan.md", nil)
	if j2.Text != "ONE\ntwo\nTHREE\nfour\n" {
		t.Errorf("document: %q", j2.Text)
	}

	// Conflicting edits keep both versions.
	if _, err := f.m.Push("s1", 1, "Plan.md", j.Doc, j2.Version, []Update{edit(19, 4, 7, "mine")}, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "Plan.md"), []byte("ONE\ntheirs\nTHREE\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.m.FileChanged("Plan.md")
	f.waitSaved("Plan.md")
	if got := f.file("Plan.md"); got != "ONE\nmine\ntheirs\nTHREE\nfour\n" {
		t.Errorf("conflict kept: %q", got)
	}
	if n := f.sink.take("s1", "notice"); len(n) == 0 || n[len(n)-1]["kind"] != "conflict" {
		t.Errorf("conflict notice: %v", n)
	}
}

func TestMoveRemoveReadOnlyAndRecover(t *testing.T) {
	f := newFixture(t, map[string]string{"A/Note.md": "text", "Other.md": "x"})
	if err := os.Symlink("Other.md", filepath.Join(f.dir, "Link.md")); err != nil {
		t.Fatal(err)
	}
	f.m.AddStream("s1", User{ID: 1, Name: "Erik"})
	j, _ := f.m.Join("s1", 1, "A/Note.md", nil)
	if _, err := f.m.Push("s1", 1, "A/Note.md", j.Doc, 0, []Update{edit(4, 4, 4, "!")}, false); err != nil {
		t.Fatal(err)
	}
	// Moving the folder saves first and keeps the document open.
	to, err := f.m.Move("A", func() (string, error) { return f.soma.Move("A", "B") })
	if err != nil || to != "B" {
		t.Fatal(to, err)
	}
	if got := f.file("B/Note.md"); got != "text!" {
		t.Errorf("saved before the move: %q", got)
	}
	if n := f.sink.take("s1", "notice"); len(n) != 1 || n[0]["kind"] != "renamed" || n[0]["to"] != "B/Note.md" {
		t.Errorf("renamed: %v", n)
	}
	if _, err := f.m.Push("s1", 1, "B/Note.md", j.Doc, 1, []Update{edit(5, 5, 5, "?")}, false); err != nil {
		t.Errorf("push after move: %v", err)
	}
	// Trash: saved, then closed.
	if err := f.m.Remove("B/Note.md", func() error { _, err := f.soma.Trash("B/Note.md"); return err }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(f.dir, ".trash", "B", "Note.md")); string(got) != "text!?" {
		t.Errorf("trashed content: %q", got)
	}
	if _, err := f.m.Push("s1", 1, "B/Note.md", j.Doc, 2, []Update{edit(6, 0, 0, "x")}, false); !errors.Is(err, ErrResync) {
		t.Errorf("push after trash: %v", err)
	}

	// Symbolic links can be followed, not changed.
	l, err := f.m.Join("s1", 1, "Link.md", nil)
	if err != nil || !l.ReadOnly {
		t.Fatalf("link: %+v %v", l, err)
	}
	if _, err := f.m.Push("s1", 1, "Link.md", l.Doc, 0, []Update{edit(1, 0, 0, "y")}, false); !errors.Is(err, soma.ErrReadOnly) {
		t.Errorf("push to a link: %v", err)
	}

	// A tab that lost its document brings its changes back in.
	o, _ := f.m.Join("s1", 1, "Other.md", nil)
	f.m.RemoveStream("s1") // closes everything soon
	time.Sleep(80 * time.Millisecond)
	f.m.AddStream("s2", User{ID: 1, Name: "Erik"})
	r, err := f.m.Join("s2", 1, "Other.md", &Recover{Doc: o.Doc, Base: "x", Mine: "x\nmine"})
	if err != nil || r.Text != "x\nmine" || r.Conflict {
		t.Errorf("recover: %+v %v", r, err)
	}
	f.waitSaved("Other.md")
}

// Several clients type at random; whoever is behind catches up and tries
// again. Everyone ends with the server's text.
func TestClientsConverge(t *testing.T) {
	f := newFixture(t, map[string]string{"Shared.md": "start\n"})
	type client struct {
		id      string
		version int
		text    []uint16
	}
	var clients []*client
	var doc string
	for i := range 4 {
		id := fmt.Sprintf("s%d", i)
		f.m.AddStream(id, User{ID: int64(i + 1), Name: id})
		j, err := f.m.Join(id, int64(i+1), "Shared.md", nil)
		if err != nil {
			t.Fatal(err)
		}
		doc = j.Doc
		clients = append(clients, &client{id: id, version: j.Version, text: fromString(j.Text)})
	}
	catchUp := func(c *client) {
		ups, v, err := f.m.Pull("Shared.md", doc, c.version)
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range ups {
			ch, _ := parseChanges(u.Changes)
			if c.text, err = ch.apply(c.text); err != nil {
				t.Fatal(err)
			}
		}
		c.version = v
	}
	rng := rand.New(rand.NewPCG(1, 2))
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Add(1)
		seed := rng.Uint64()
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewPCG(uint64(i), seed))
			for n := 0; n < 40; {
				l := len(c.text)
				from := r.IntN(l + 1)
				to := min(l, from+r.IntN(3))
				u := edit(l, from, to, []string{"a", "ü", "😀", "\n", ""}[r.IntN(5)])
				_, err := f.m.Push(c.id, int64(i+1), "Shared.md", doc, c.version, []Update{u}, false)
				switch {
				case errors.Is(err, ErrVersion):
					catchUp(c)
				case errors.Is(err, ErrRateLimited):
					time.Sleep(20 * time.Millisecond)
				case err != nil:
					t.Error(err)
					return
				default:
					catchUp(c)
					n++
				}
			}
		}()
	}
	wg.Wait()
	final, _ := f.m.Join("s0", 1, "Shared.md", nil)
	for _, c := range clients {
		catchUp(c)
		if toString(c.text) != final.Text {
			t.Errorf("%s diverged", c.id)
		}
	}
	f.m.Flush("")
	if got := f.file("Shared.md"); got != final.Text {
		t.Errorf("file differs from the document")
	}
}
