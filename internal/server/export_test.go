package server

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func zipNames(t *testing.T, body []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	return names
}

func TestExportAndBackup(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	cara, caraUser := newUser(t, s, admin, "cara")
	base := "/api/somas/" + testSoma
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, caraUser.ID), `{"role":"viewer"}`), 200, "")
	expect(t, admin.do("POST", base+"/notes/Garden/Beds.md", `{"content":"tomatoes"}`), 201, "")
	expect(t, admin.do("POST", base+"/folders/Empty", ``), 201, "")
	expect(t, admin.do("POST", base+"/notes/Gone.md", `{"content":"x"}`), 201, "")
	expect(t, admin.do("DELETE", base+"/entries/Gone.md", ``), 200, "")
	for _, p := range []string{".obsidian/app.json"} {
		os.MkdirAll(filepath.Dir(filepath.Join(somaDir(s), p)), 0o755)
		os.WriteFile(filepath.Join(somaDir(s), p), []byte("{}"), 0o644)
	}

	// Everyone with access downloads the visible files.
	rec := cara.do("GET", base+"/export", "")
	expect(t, rec, 200, "")
	if h := rec.Header(); h.Get("Content-Type") != "application/zip" || !strings.Contains(h.Get("Content-Disposition"), `filename=Test.zip`) {
		t.Errorf("headers: %v", h)
	}
	names := zipNames(t, rec.Body.Bytes())
	for _, want := range []string{"Empty/", "Garden/", "Garden/Beds.md"} {
		if !slices.Contains(names, want) {
			t.Errorf("%s missing from %v", want, names)
		}
	}
	for _, n := range names {
		if strings.HasPrefix(n, ".") || strings.Contains(n, "Gone") {
			t.Errorf("hidden entry exported: %s", n)
		}
	}

	// The database backup is for admins.
	expect(t, cara.do("GET", "/api/admin/backup", ""), 403, "")
	rec = admin.do("GET", "/api/admin/backup", "")
	expect(t, rec, 200, "")
	if !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment; filename=dendronotes-backup-") {
		t.Errorf("backup headers: %v", rec.Header())
	}
	names = zipNames(t, rec.Body.Bytes())
	if !slices.Equal(names, []string{"history/" + testSoma + ".db", "manifest.json", "state.db"}) {
		t.Errorf("backup contents: %v", names)
	}
}
