package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dendronotes/internal/history"
	"dendronotes/internal/store"
)

type versionList struct {
	Versions []history.Version
	People   map[string]store.Person
}

func TestHistory(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	cara, caraUser := newUser(t, s, admin, "cara")
	base := "/api/somas/" + testSoma
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, benUser.ID), `{"role":"writer"}`), 200, "")
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, caraUser.ID), `{"role":"viewer"}`), 200, "")

	rec := admin.do("POST", base+"/notes/Plan.md", `{"content":"beds"}`)
	expect(t, rec, 201, "")
	etag := decode[map[string]any](t, rec)["etag"].(string)
	save := func(c *client, content string) {
		t.Helper()
		rec := c.do("PUT", base+"/notes/Plan.md", fmt.Sprintf(`{"content":%q}`, content), "If-Match", `"`+etag+`"`)
		expect(t, rec, 200, "")
		etag = decode[map[string]any](t, rec)["etag"].(string)
	}
	save(admin, "beds and paths")
	save(admin, "beds, paths") // same session: one version
	save(ben, "beds, paths, a pond")
	list := func(c *client, p string) versionList {
		t.Helper()
		rec := c.do("GET", base+"/history/"+p, "")
		expect(t, rec, 200, "")
		return decode[versionList](t, rec)
	}

	// Everyone with access reads the history, with the people in it.
	got := list(cara, "Plan.md")
	if len(got.Versions) != 3 || got.Versions[0].Reason != "edit" || got.Versions[2].Reason != "create" {
		t.Fatalf("versions: %+v", got.Versions)
	}
	if p, ok := got.People[fmt.Sprint(benUser.ID)]; !ok || p.Username != "ben" || p.Avatar == nil {
		t.Errorf("people: %+v", got.People)
	}
	created := got.Versions[2].ID
	rec = cara.do("GET", fmt.Sprintf("%s/versions/%d", base, created), "")
	if expect(t, rec, 200, ""); decode[map[string]any](t, rec)["content"] != "beds" {
		t.Errorf("version content: %s", rec.Body)
	}
	expect(t, cara.do("GET", base+"/versions/999", ""), 404, "not_found")
	expect(t, cara.do("GET", base+"/versions/x", ""), 404, "not_found")

	// Writers bring an earlier version back; readers cannot.
	restore := fmt.Sprintf("%s/versions/%d/restore", base, created)
	expect(t, cara.do("POST", restore, ``), 403, "forbidden")
	expect(t, ben.do("POST", restore, ``), 200, "")
	rec = admin.do("GET", base+"/notes/Plan.md", "")
	if decode[map[string]any](t, rec)["content"] != "beds" {
		t.Errorf("after restore: %s", rec.Body)
	}
	got = list(admin, "Plan.md")
	if len(got.Versions) != 4 || got.Versions[0].Reason != "restore" || got.Versions[0].RestoredFrom != created ||
		len(got.Versions[0].Authors) != 1 || got.Versions[0].Authors[0] != benUser.ID {
		t.Errorf("after restore: %+v", got.Versions)
	}

	// The history follows renames, goes to the trash and comes back.
	expect(t, admin.do("POST", base+"/move", `{"from":"Plan.md","to":"Garden plan.md"}`), 200, "")
	if n := len(list(admin, "Garden%20plan.md").Versions); n != 4 {
		t.Errorf("after rename: %d versions", n)
	}
	if n := len(list(admin, "Plan.md").Versions); n != 0 {
		t.Errorf("old name keeps %d versions", n)
	}
	expect(t, admin.do("DELETE", base+"/entries/Garden%20plan.md", ``), 200, "")
	if n := len(list(admin, "Garden%20plan.md").Versions); n != 0 {
		t.Errorf("trashed note shows %d versions", n)
	}
	expect(t, admin.do("GET", fmt.Sprintf("%s/versions/%d", base, created), ""), 404, "not_found") // in the trash now
	expect(t, admin.do("POST", base+"/trash/Garden%20plan.md", ``), 200, "")
	if n := len(list(admin, "Garden%20plan.md").Versions); n != 4 {
		t.Errorf("after restoring from the trash: %d versions", n)
	}
	// Purged notes lose their history.
	expect(t, admin.do("DELETE", base+"/entries/Garden%20plan.md", ``), 200, "")
	expect(t, admin.do("DELETE", base+"/trash", ``), 200, "")
	expect(t, admin.do("POST", base+"/notes/Garden%20plan.md", `{"content":""}`), 201, "")
	if n := len(list(admin, "Garden%20plan.md").Versions); n != 1 {
		t.Errorf("a new note with the old name has %d versions", n)
	}
}

// Notes changed outside dendronotes are recorded when the watcher sees them.
func TestHistoryOutside(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	base := "/api/somas/" + testSoma
	expect(t, admin.do("POST", base+"/notes/Log.md", `{"content":"one"}`), 201, "")
	rt := rtOf(t, s)
	waitWatching(t, rt)
	if err := os.WriteFile(filepath.Join(somaDir(s), "Log.md"), []byte("one\ntwo"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		list, err := rt.History.List(context.Background(), "Log.md")
		if err == nil && len(list) == 2 && list[0].Reason == history.ReasonOutside {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("outside change not recorded: %+v %v", list, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
