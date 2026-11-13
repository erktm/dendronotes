package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dendronotes/internal/store"
)

func TestFolderOrderFollowsTheFolder(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	c.login()
	dir := somaDir(s)
	for _, p := range []string{"Beds/North.md", "Beds/South.md", "Plan.md"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte("# "+p), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := "/api/somas/" + testSoma
	expect(t, c.do("PUT", base+"/order", `{"folder":"Beds","mode":"manual","names":["South.md","North.md"]}`), 204, "")
	expect(t, c.do("PUT", base+"/order", `{"folder":"","mode":"created"}`), 204, "")
	expect(t, c.do("PUT", base+"/order", `{"folder":"Plan.md","mode":"manual"}`), 400, "not_a_folder")
	expect(t, c.do("PUT", base+"/order", `{"folder":"Gone","mode":"manual"}`), 404, "not_found")
	expect(t, c.do("PUT", base+"/order", `{"folder":"../x","mode":"manual"}`), 400, "invalid_path")
	expect(t, c.do("PUT", base+"/order", `{"folder":"Beds","mode":"random"}`), 400, "invalid_mode")
	expect(t, c.do("PUT", base+"/order", `{"folder":"Beds","mode":"manual","names":["a/b"]}`), 400, "invalid_name")

	type tree struct {
		Entries []struct {
			Path  string `json:"path"`
			CTime int64  `json:"ctime"`
		} `json:"entries"`
		Orders map[string]store.FolderOrder `json:"orders"`
	}
	got := decode[tree](t, c.do("GET", base+"/tree", ""))
	if o := got.Orders["Beds"]; o.Mode != "manual" || len(o.Names) != 2 || o.Names[0] != "South.md" {
		t.Fatalf("orders = %v", got.Orders)
	}
	if got.Orders[""].Mode != "created" {
		t.Errorf("top level = %v", got.Orders[""])
	}
	for _, e := range got.Entries {
		if e.CTime <= 0 || e.CTime > time.Now().Add(time.Minute).UnixMilli() {
			t.Errorf("%s: ctime %d", e.Path, e.CTime)
		}
	}

	// The order goes along with a rename, into the trash and back.
	expect(t, c.do("POST", base+"/move", `{"from":"Beds","to":"Garden beds"}`), 200, "")
	got = decode[tree](t, c.do("GET", base+"/tree", ""))
	if _, ok := got.Orders["Beds"]; ok || got.Orders["Garden beds"].Mode != "manual" {
		t.Fatalf("after the rename: %v", got.Orders)
	}
	expect(t, c.do("DELETE", base+"/entries/Garden%20beds", ""), 200, "")
	got = decode[tree](t, c.do("GET", base+"/tree", ""))
	if len(got.Orders) != 1 {
		t.Errorf("orders in the trash are listed: %v", got.Orders)
	}
	expect(t, c.do("POST", base+"/trash/Garden%20beds", ""), 200, "")
	got = decode[tree](t, c.do("GET", base+"/tree", ""))
	if got.Orders["Garden beds"].Mode != "manual" {
		t.Errorf("after restoring: %v", got.Orders)
	}

	// Sorting by name again forgets the folder.
	expect(t, c.do("PUT", base+"/order", `{"folder":"Garden beds","mode":"name"}`), 204, "")
	got = decode[tree](t, c.do("GET", base+"/tree", ""))
	if _, ok := got.Orders["Garden beds"]; ok {
		t.Errorf("name order is stored: %v", got.Orders)
	}
}
