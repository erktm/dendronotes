package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dendronotes/internal/collab"
	"dendronotes/internal/store"
)

// TestEditingTogether runs the whole round trip: the event stream names the
// tab, the tab joins a note, pushes a change, receives it back on the stream
// and the note is saved.
func TestEditingTogether(t *testing.T) {
	s := newTestServer(t, testFS())
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	c := newClient(t, s)
	c.login()
	if err := os.WriteFile(filepath.Join(somaDir(s), "Plan.md"), []byte("Beds\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	call := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.AddCookie(c.cookie)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		b, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(b, &out)
		return resp.StatusCode, out
	}

	req, _ := http.NewRequest("GET", srv.URL+"/api/somas/testsoma01/events", nil)
	req.AddCookie(c.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := bufio.NewScanner(resp.Body)
	next := func(event string) map[string]any {
		t.Helper()
		for lines.Scan() {
			if lines.Text() != "event: "+event {
				continue
			}
			lines.Scan()
			var m map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(lines.Text(), "data: ")), &m); err != nil {
				t.Fatal(err)
			}
			return m
		}
		t.Fatalf("stream ended before %s", event)
		return nil
	}
	stream, _ := next("hello")["stream"].(string)
	if len(stream) < 16 {
		t.Fatalf("hello without stream id")
	}

	code, j := call("POST", "/api/somas/testsoma01/collab/join", fmt.Sprintf(`{"stream":%q,"path":"Plan.md"}`, stream))
	if code != 200 || j["text"] != "Beds\n" || j["readOnly"] != false {
		t.Fatalf("join: %d %v", code, j)
	}
	doc := j["doc"].(string)
	next("presence")

	push := func(version int, changes string) (int, map[string]any) {
		return call("POST", "/api/somas/testsoma01/collab/push", fmt.Sprintf(
			`{"stream":%q,"path":"Plan.md","doc":%q,"version":%d,"updates":[{"clientID":"t1","changes":%s}],"save":true}`,
			stream, doc, version, changes))
	}
	if code, r := push(0, `[4,[0," and paths"],1]`); code != 200 || r["version"] != 1.0 {
		t.Fatalf("push: %d %v", code, r)
	}
	if e := next("collab"); e["from"] != 0.0 || e["path"] != "Plan.md" {
		t.Errorf("collab event: %v", e)
	}
	if e := next("saved"); e["version"] != 1.0 {
		t.Errorf("saved event: %v", e)
	}
	if b, _ := os.ReadFile(filepath.Join(somaDir(s), "Plan.md")); string(b) != "Beds and paths\r\n" {
		t.Errorf("file: %q", b)
	}

	if code, r := push(0, `[[0,"x"],15]`); code != 409 || r["error"] != "version_mismatch" || r["version"] != 1.0 {
		t.Errorf("stale push: %d %v", code, r)
	}
	if code, r := push(1, `[3]`); code != 400 || r["error"] != "invalid_update" {
		t.Errorf("invalid push: %d %v", code, r)
	}
	if code, r := call("GET", "/api/somas/testsoma01/collab/pull?path=Plan.md&doc="+doc+"&version=0", ""); code != 200 || len(r["updates"].([]any)) != 1 {
		t.Errorf("pull: %d %v", code, r)
	}
	if code, r := call("GET", "/api/somas/testsoma01/collab/pull?path=Plan.md&doc=old&version=0", ""); code != 410 || r["error"] != "resync" {
		t.Errorf("pull of an old document: %d %v", code, r)
	}
	if code, r := call("POST", "/api/somas/testsoma01/collab/join", `{"stream":"made-up","path":"Plan.md"}`); code != 409 || r["error"] != "unknown_stream" {
		t.Errorf("unknown stream: %d %v", code, r)
	}
	if code, _ := call("POST", "/api/somas/testsoma01/collab/presence", fmt.Sprintf(`{"stream":%q,"path":"Plan.md","doc":%q,"sel":{"anchor":2,"head":2}}`, stream, doc)); code != 200 {
		t.Errorf("presence: %d", code)
	}

	// Renaming the note carries the open document along.
	code, _ = call("POST", "/api/somas/testsoma01/move", `{"from":"Plan.md","to":"Garden plan.md"}`)
	if code != 200 {
		t.Fatalf("move: %d", code)
	}
	if e := next("notice"); e["kind"] != "renamed" || e["to"] != "Garden plan.md" {
		t.Errorf("rename notice: %v", e)
	}
	if code, _ := call("POST", "/api/somas/testsoma01/collab/leave", fmt.Sprintf(`{"stream":%q,"path":"Garden plan.md"}`, stream)); code != 200 {
		t.Errorf("leave: %d", code)
	}
}

func TestViewersFollowAlong(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	expect(t, admin.do("POST", "/api/somas/"+testSoma+"/notes/Plan.md", `{"content":"x"}`), 201, "")
	if err := s.store.SetMember(t.Context(), testSoma, benUser.ID, store.MemberViewer); err != nil {
		t.Fatal(err)
	}
	rt := rtOf(t, s)
	tab, events, cancel, _ := rt.Hub.Subscribe()
	defer cancel()
	rt.Collab.AddStream(tab, collab.User{ID: benUser.ID, Name: "ben"})
	rec := ben.do("POST", "/api/somas/"+testSoma+"/collab/join", fmt.Sprintf(`{"stream":%q,"path":"Plan.md"}`, tab))
	expect(t, rec, 200, "")
	j := decode[collab.Joined](t, rec)
	if !j.ReadOnly || j.Text != "x" {
		t.Errorf("viewer join: %+v", j)
	}
	expect(t, ben.do("POST", "/api/somas/"+testSoma+"/collab/push", fmt.Sprintf(
		`{"stream":%q,"path":"Plan.md","doc":%q,"version":0,"updates":[]}`, tab, j.Doc)), 403, "forbidden")
	// Taking ben's access away ends his stream at once.
	expect(t, admin.do("DELETE", fmt.Sprintf("/api/somas/%s/members/%d", testSoma, benUser.ID), ""), 200, "")
	for range events {
	}
}
