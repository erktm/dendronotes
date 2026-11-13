package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	expect(t, c.do("PUT", base+"/order", `{"folder":"Beds","names":["South.md","North.md"]}`), 204, "")
	expect(t, c.do("PUT", base+"/order", `{"folder":"","names":["Plan.md","Beds"]}`), 204, "")
	expect(t, c.do("PUT", base+"/order", `{"folder":"Plan.md","names":["x"]}`), 400, "not_a_folder")
	expect(t, c.do("PUT", base+"/order", `{"folder":"Gone","names":["x"]}`), 404, "not_found")
	expect(t, c.do("PUT", base+"/order", `{"folder":"../x","names":["x"]}`), 400, "invalid_path")
	expect(t, c.do("PUT", base+"/order", `{"folder":"Beds","names":["a/b"]}`), 400, "invalid_name")

	type tree struct {
		Entries []struct {
			Path  string `json:"path"`
			CTime int64  `json:"ctime"`
		} `json:"entries"`
		Orders map[string][]string `json:"orders"`
	}
	got := decode[tree](t, c.do("GET", base+"/tree", ""))
	if o := got.Orders["Beds"]; len(o) != 2 || o[0] != "South.md" {
		t.Fatalf("orders = %v", got.Orders)
	}
	if o := got.Orders[""]; len(o) != 2 || o[0] != "Plan.md" {
		t.Errorf("top level = %v", o)
	}
	for _, e := range got.Entries {
		if e.CTime <= 0 || e.CTime > time.Now().Add(time.Minute).UnixMilli() {
			t.Errorf("%s: ctime %d", e.Path, e.CTime)
		}
	}

	// The order goes along with a rename, into the trash and back.
	expect(t, c.do("POST", base+"/move", `{"from":"Beds","to":"Garden beds"}`), 200, "")
	got = decode[tree](t, c.do("GET", base+"/tree", ""))
	if _, ok := got.Orders["Beds"]; ok || len(got.Orders["Garden beds"]) != 2 || got.Orders[""][1] != "Garden beds" {
		t.Fatalf("after the rename: %v", got.Orders)
	}
	expect(t, c.do("DELETE", base+"/entries/Garden%20beds", ""), 200, "")
	got = decode[tree](t, c.do("GET", base+"/tree", ""))
	if len(got.Orders) != 1 {
		t.Errorf("orders in the trash are listed: %v", got.Orders)
	}
	expect(t, c.do("POST", base+"/trash/Garden%20beds", ""), 200, "")
	got = decode[tree](t, c.do("GET", base+"/tree", ""))
	if len(got.Orders["Garden beds"]) != 2 {
		t.Errorf("after restoring: %v", got.Orders)
	}

	// No names forgets the folder.
	expect(t, c.do("PUT", base+"/order", `{"folder":"Garden beds","names":[]}`), 204, "")
	got = decode[tree](t, c.do("GET", base+"/tree", ""))
	if _, ok := got.Orders["Garden beds"]; ok {
		t.Errorf("an empty order is stored: %v", got.Orders)
	}
}
