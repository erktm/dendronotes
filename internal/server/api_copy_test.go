package server

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"dendronotes/internal/drawing"
	"dendronotes/internal/soma"
	"dendronotes/internal/store"
)

func TestCopyToAnotherSoma(t *testing.T) {
	s := newIndexedServer(t, map[string]string{
		"Garden/Plan.md":            "Beds in rows",
		"Garden/Seeds/List.md":      "Beans",
		"Garden/Beds.drawing.svg":   string(drawing.Empty()),
		"Garden/broken.drawing.svg": `<svg xmlns="http://www.w3.org/2000/svg"><script>x</script></svg>`,
		"Garden/photo.png":          pngBytes,
		"Garden/tool.exe":           "MZ program",
		"Garden/.hidden.md":         "never",
	})
	admin := newClient(t, s)
	admin.login()
	base := "/api/somas/" + testSoma
	rec := admin.do("POST", "/api/somas", `{"name":"Other"}`)
	expect(t, rec, 201, "")
	other := decode[store.Membership](t, rec).ID
	copyTo := func(c *client, from, target, to string) *copyResult {
		t.Helper()
		rec := c.do("POST", base+"/copy", fmt.Sprintf(`{"from":%q,"soma":%q,"to":%q}`, from, target, to))
		if rec.Code != 201 {
			t.Fatalf("copy %s to %s/%s: %d %s", from, target, to, rec.Code, rec.Body)
		}
		r := decode[copyResult](t, rec)
		return &r
	}
	tree := func(id string) []string {
		var out []string
		for _, e := range decode[struct{ Entries []soma.Entry }](t, admin.do("GET", "/api/somas/"+id+"/tree", "")).Entries {
			out = append(out, e.Path)
		}
		sort.Strings(out)
		return out
	}

	// A note lands at the top; a taken name gets a suffix.
	if r := copyTo(admin, "Garden/Plan.md", other, ""); r.Path != "Plan.md" || r.Files != 1 {
		t.Errorf("note: %+v", r)
	}
	if r := copyTo(admin, "Garden/Plan.md", other, ""); r.Path != "Plan (1).md" {
		t.Errorf("second copy: %+v", r)
	}
	if n := decode[struct{ Content string }](t, admin.do("GET", "/api/somas/"+other+"/notes/Plan.md", "")); n.Content != "Beds in rows" {
		t.Errorf("copied note: %+v", n)
	}
	if v := decode[struct{ Versions []struct{ Reason string } }](t, admin.do("GET", "/api/somas/"+other+"/history/Plan.md", "")).Versions; len(v) != 1 || v[0].Reason != "create" {
		t.Errorf("a copied note starts a history: %+v", v)
	}

	// A folder with everything inside; what the soma would not take as an
	// upload is left out and named, hidden entries never come along.
	r := copyTo(admin, "Garden", other, "")
	if r.Path != "Garden" || r.Folders != 2 || r.Files != 4 || len(r.Skipped) != 2 {
		t.Errorf("folder: %+v", r)
	}
	var skipped []string
	for _, sk := range r.Skipped {
		skipped = append(skipped, sk.File)
	}
	sort.Strings(skipped)
	if strings.Join(skipped, ",") != "Garden/broken.drawing.svg,Garden/tool.exe" {
		t.Errorf("skipped: %v", r.Skipped)
	}
	want := "Garden,Garden/Beds.drawing.svg,Garden/Plan.md,Garden/Seeds,Garden/Seeds/List.md,Garden/photo.png,Plan (1).md,Plan.md"
	if got := strings.Join(tree(other), ","); got != want {
		t.Errorf("other soma:\n got %s\nwant %s", got, want)
	}

	// Into a folder of the same soma, even into itself: the copy is planned
	// before anything is written.
	if r := copyTo(admin, "Garden", testSoma, "Garden"); r.Path != "Garden/Garden" {
		t.Errorf("into itself: %+v", r)
	}
	for _, p := range tree(testSoma) {
		if strings.HasPrefix(p, "Garden/Garden/Garden") {
			t.Errorf("copied its own copy: %s", p)
		}
	}

	// Wrong places and things.
	expect(t, admin.do("POST", base+"/copy", fmt.Sprintf(`{"from":"Garden","soma":%q,"to":"Plan.md"}`, other)), 400, "not_a_directory")
	expect(t, admin.do("POST", base+"/copy", fmt.Sprintf(`{"from":"Garden","soma":%q,"to":"Nowhere"}`, other)), 404, "not_found")
	expect(t, admin.do("POST", base+"/copy", fmt.Sprintf(`{"from":"Missing.md","soma":%q}`, other)), 404, "not_found")
	expect(t, admin.do("POST", base+"/copy", fmt.Sprintf(`{"from":"Garden/.hidden.md","soma":%q}`, other)), 400, "invalid_path")
	expect(t, admin.do("POST", base+"/copy", fmt.Sprintf(`{"from":"Garden/tool.exe","soma":%q}`, other)), 422, "not_copied")
	expect(t, admin.do("POST", base+"/copy", `{"from":"Garden/Plan.md","soma":"nosuchsoma"}`), 404, "not_found")

	// The target needs contributor: without access it does not exist.
	ben, benUser := newUser(t, s, admin, "ben")
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, benUser.ID), `{"role":"viewer"}`), 200, "")
	body := fmt.Sprintf(`{"from":"Garden/Plan.md","soma":%q}`, other)
	expect(t, ben.do("POST", base+"/copy", body), 404, "not_found")
	expect(t, admin.do("PUT", fmt.Sprintf("/api/somas/%s/members/%d", other, benUser.ID), `{"role":"writer"}`), 200, "")
	expect(t, ben.do("POST", base+"/copy", body), 403, "forbidden")
	expect(t, admin.do("PUT", fmt.Sprintf("/api/somas/%s/members/%d", other, benUser.ID), `{"role":"contributor"}`), 200, "")
	if r := copyTo(ben, "Garden/Plan.md", other, ""); r.Path != "Plan (2).md" {
		t.Errorf("a viewer of the source copies into a soma they contribute to: %+v", r)
	}
	// Viewing the source is enough to copy out of it, never into it.
	expect(t, ben.do("POST", "/api/somas/"+other+"/copy", fmt.Sprintf(`{"from":"Plan.md","soma":%q}`, testSoma)), 403, "forbidden")
}
