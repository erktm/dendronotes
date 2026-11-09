package server

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dendronotes/internal/store"
)

type trashList struct {
	Items []struct {
		Path      string
		Original  string
		Type      string
		Files     int
		DeletedBy *store.Person
	}
}

func TestTrash(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	cara, caraUser := newUser(t, s, admin, "cara")
	base := "/api/somas/" + testSoma
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, benUser.ID), `{"role":"editor"}`), 200, "")
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, caraUser.ID), `{"role":"contributor"}`), 200, "")
	expect(t, admin.do("POST", base+"/notes/Garden/Beds.md", `{"content":"tomatoes"}`), 201, "")
	expect(t, admin.do("POST", base+"/notes/Ideas.md", `{"content":"ideas"}`), 201, "")
	list := func(c *client) trashList {
		t.Helper()
		rec := c.do("GET", base+"/trash", "")
		expect(t, rec, 200, "")
		return decode[trashList](t, rec)
	}

	// Who deleted what, from where; only editors and owners see it.
	expect(t, ben.do("DELETE", base+"/entries/Garden/Beds.md", ``), 200, "")
	got := list(ben)
	if len(got.Items) != 1 || got.Items[0].Path != "Garden/Beds.md" || got.Items[0].Original != "Garden/Beds.md" ||
		got.Items[0].DeletedBy == nil || got.Items[0].DeletedBy.Username != "ben" {
		t.Fatalf("trash: %+v", got)
	}
	expect(t, cara.do("GET", base+"/trash", ""), 403, "forbidden")
	expect(t, cara.do("POST", base+"/trash/Garden/Beds.md", ``), 403, "forbidden")
	rec := ben.do("GET", base+"/trash/Garden/Beds.md", "")
	if expect(t, rec, 200, ""); decode[map[string]any](t, rec)["content"] != "tomatoes" {
		t.Errorf("preview: %s", rec.Body)
	}
	expect(t, ben.do("GET", base+"/trash/..%2FIdeas.md", ""), 400, "invalid_path")

	// Restoring puts it back; an occupied place gets a suffix.
	expect(t, admin.do("POST", base+"/notes/Garden/Beds.md", `{"content":"new beds"}`), 201, "")
	rec = ben.do("POST", base+"/trash/Garden/Beds.md", ``)
	if expect(t, rec, 200, ""); decode[map[string]string](t, rec)["path"] != "Garden/Beds (1).md" {
		t.Errorf("restored to %s", rec.Body)
	}
	expect(t, ben.do("GET", base+"/notes/"+url.PathEscape("Garden/Beds (1).md"), ""), 200, "")
	if got := list(ben); len(got.Items) != 0 {
		t.Errorf("trash after restore: %+v", got)
	}

	// A folder is one entry; what is deleted later lands next to it.
	expect(t, ben.do("DELETE", base+"/entries/Garden", ``), 200, "")
	expect(t, admin.do("POST", base+"/notes/Garden/Later.md", `{"content":"later"}`), 201, "")
	expect(t, ben.do("DELETE", base+"/entries/Garden/Later.md", ``), 200, "")
	// What Obsidian put into the trash shows up too.
	if err := os.WriteFile(filepath.Join(somaDir(s), ".trash", "From Obsidian.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = list(ben)
	byPath := map[string]int{}
	for i, it := range got.Items {
		byPath[it.Path] = i
	}
	if len(got.Items) != 3 || got.Items[byPath["Garden"]].Type != "dir" || got.Items[byPath["Garden"]].Files != 2 {
		t.Fatalf("trash with a folder: %+v", got)
	}
	if it := got.Items[byPath["Garden (1)/Later.md"]]; it.Original != "Garden/Later.md" {
		t.Errorf("later note: %+v", it)
	}
	if it := got.Items[byPath["From Obsidian.md"]]; it.DeletedBy != nil || it.Original != "From Obsidian.md" {
		t.Errorf("unrecorded entry: %+v", it)
	}

	// Deleting for good is for the owner.
	expect(t, ben.do("DELETE", base+"/trash/Garden", ``), 403, "forbidden")
	expect(t, ben.do("DELETE", base+"/trash", ``), 403, "forbidden")
	expect(t, admin.do("DELETE", base+"/trash/Garden", ``), 200, "")
	if got := list(admin); len(got.Items) != 2 {
		t.Errorf("after purging the folder: %+v", got)
	}
	expect(t, admin.do("DELETE", base+"/trash", ``), 200, "")
	if got := list(admin); len(got.Items) != 0 {
		t.Errorf("after emptying: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(somaDir(s), "Ideas.md")); err != nil {
		t.Errorf("emptying the trash touched the soma: %v", err)
	}
}

func TestDeletedSomas(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, _ := newUser(t, s, admin, "ben")
	cara, caraUser := newUser(t, s, admin, "cara")

	rec := ben.do("POST", "/api/somas", `{"name":"Seeds"}`)
	expect(t, rec, 201, "")
	seeds := decode[store.Soma](t, rec)
	base := "/api/somas/" + seeds.ID
	expect(t, ben.do("POST", base+"/notes/List.md", `{"content":"beans"}`), 201, "")
	expect(t, ben.do("PUT", fmt.Sprintf("%s/members/%d", base, caraUser.ID), `{"role":"viewer"}`), 200, "")
	expect(t, ben.do("DELETE", base, ``), 200, "")
	expect(t, cara.do("GET", base+"/tree", ""), 404, "not_found")

	deleted := func() []store.DeletedSoma {
		t.Helper()
		rec := admin.do("GET", "/api/admin/somas/deleted", "")
		expect(t, rec, 200, "")
		var out []store.DeletedSoma // only this test's soma (ben's own soma joins it below)
		for _, d := range decode[struct{ Somas []store.DeletedSoma }](t, rec).Somas {
			if d.ID == seeds.ID {
				out = append(out, d)
			}
		}
		return out
	}
	list := deleted()
	if len(list) != 1 || list[0].ID != seeds.ID || list[0].Owner == nil || list[0].Owner.Username != "ben" || list[0].Members != 1 {
		t.Fatalf("deleted somas: %+v", list)
	}
	expect(t, ben.do("GET", "/api/admin/somas/deleted", ""), 403, "")
	restore := "/api/admin/somas/deleted/" + seeds.ID + "/restore"

	// Back with its notes and shares.
	expect(t, admin.do("POST", restore, ``), 200, "")
	rec = cara.do("GET", base+"/notes/List.md", "")
	if expect(t, rec, 200, ""); decode[map[string]any](t, rec)["content"] != "beans" {
		t.Errorf("restored note: %s", rec.Body)
	}
	expect(t, admin.do("POST", restore, ``), 404, "not_found")

	// Deleted again, and the owner's account is gone: the admin picks one.
	expect(t, ben.do("DELETE", base, ``), 200, "")
	benID := deleted()[0].Owner.ID
	expect(t, admin.do("DELETE", fmt.Sprintf("/api/admin/users/%d", benID), ``), 200, "")
	if list := deleted(); len(list) != 1 || list[0].Owner != nil {
		t.Fatalf("owner gone: %+v", list)
	}
	expect(t, admin.do("POST", restore, ``), 400, "owner_required")
	expect(t, admin.do("POST", restore, `{"ownerId":9999}`), 400, "unknown_user")
	expect(t, admin.do("POST", restore, fmt.Sprintf(`{"ownerId":%d}`, caraUser.ID)), 200, "")
	somasOfCara := decode[struct{ Somas []store.Membership }](t, cara.do("GET", "/api/somas", "")).Somas
	found := false
	for _, m := range somasOfCara {
		if m.ID == seeds.ID && m.Role == "owner" {
			found = true
		}
	}
	if !found {
		t.Errorf("cara does not own the restored soma: %+v", somasOfCara)
	}

	// For good: the files go.
	expect(t, cara.do("DELETE", base, ``), 200, "")
	folder := deleted()[0].Folder
	expect(t, admin.do("DELETE", "/api/admin/somas/deleted/"+seeds.ID, ``), 200, "")
	if _, err := os.Stat(filepath.Join(s.cfg.SomasDir, folder)); !os.IsNotExist(err) {
		t.Errorf("files of the purged soma: %v", err)
	}
	if list := deleted(); len(list) != 0 {
		t.Errorf("after purging: %+v", list)
	}
}

// Entries are deleted for good after TRASH_DAYS – counted for files put into
// the trash elsewhere from when dendronotes first sees them.
func TestTrashExpires(t *testing.T) {
	s := newTestServer(t, testFS())
	s.cfg.TrashDays = 7
	admin := newClient(t, s)
	admin.login()
	base := "/api/somas/" + testSoma
	expect(t, admin.do("POST", base+"/notes/Old.md", `{"content":"old"}`), 201, "")
	expect(t, admin.do("DELETE", base+"/entries/Old.md", ``), 200, "")
	// Obsidian moved this one long ago (its file time is old).
	obsidian := filepath.Join(somaDir(s), ".trash", "From Obsidian.md")
	if err := os.WriteFile(obsidian, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, -2, 0)
	if err := os.Chtimes(obsidian, old, old); err != nil {
		t.Fatal(err)
	}
	rec := admin.do("GET", base+"/trash", "")
	expect(t, rec, 200, "")
	if got := decode[struct {
		Items    []map[string]any
		KeepDays int
	}](t, rec); len(got.Items) != 2 || got.KeepDays != 7 {
		t.Fatalf("trash: %+v", got)
	}

	rt := rtOf(t, s)
	ctx := context.Background()
	if n, err := s.expireTrash(ctx, rt, time.Now().AddDate(0, 0, 6)); err != nil || n != 0 {
		t.Errorf("after 6 days: %d %v", n, err)
	}
	if n, err := s.expireTrash(ctx, rt, time.Now().AddDate(0, 0, 8)); err != nil || n != 2 {
		t.Errorf("after 8 days: %d %v", n, err)
	}
	if _, err := os.Stat(obsidian); !os.IsNotExist(err) {
		t.Errorf("expired file still there: %v", err)
	}
	if got := decode[trashList](t, admin.do("GET", base+"/trash", "")); len(got.Items) != 0 {
		t.Errorf("trash after expiry: %+v", got)
	}

	// 0 keeps everything.
	s.cfg.TrashDays = 0
	expect(t, admin.do("POST", base+"/notes/Keep.md", `{"content":"k"}`), 201, "")
	expect(t, admin.do("DELETE", base+"/entries/Keep.md", ``), 200, "")
	if n, _ := s.expireTrash(ctx, rt, time.Now().AddDate(1, 0, 0)); n != 0 {
		t.Errorf("expired with TRASH_DAYS=0: %d", n)
	}
}
