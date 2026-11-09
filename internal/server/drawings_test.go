package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"dendronotes/internal/drawing"
	"dendronotes/internal/store"
)

func svgBody(svg string) string {
	b, _ := json.Marshal(map[string]string{"svg": svg})
	return string(b)
}

const savedDrawing = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100" width="200" height="100">` +
	`<metadata id="dendronotes-drawing">{"version":1,"elements":[{"id":"a","type":"rect"}]}</metadata>` +
	`<rect x="10" y="10" width="80" height="40" fill="none" stroke="#1f2328"/></svg>`

func TestDrawings(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	base := "/api/somas/" + testSoma
	url := base + "/drawings/Plans/Garden.drawing.svg"

	// Creating writes an empty drawing; a taken name fails or gets a suffix.
	rec := admin.do("POST", url, "")
	expect(t, rec, 201, "")
	created := decode[drawingFile](t, rec)
	if created.Path != "Plans/Garden.drawing.svg" || created.ETag == "" || created.SVG != "" {
		t.Fatalf("created: %+v", created)
	}
	expect(t, admin.do("POST", url, ""), 409, "")
	if f := decode[drawingFile](t, admin.do("POST", url+"?unique=1", "")); f.Path != "Plans/Garden (1).drawing.svg" {
		t.Errorf("unique: %+v", f)
	}
	expect(t, admin.do("POST", base+"/drawings/Plans/Garden.svg", ""), 400, "not_a_drawing")
	expect(t, admin.do("POST", base+"/notes/Plans/Note.md", `{"content":"x"}`), 201, "")
	expect(t, admin.do("GET", base+"/drawings/Plans/Note.md", ""), 400, "not_a_drawing")
	expect(t, admin.do("GET", base+"/drawings/Plans/Missing.drawing.svg", ""), 404, "")

	// Reading returns the SVG and its ETag.
	rec = admin.do("GET", url, "")
	expect(t, rec, 200, "")
	read := decode[drawingFile](t, rec)
	if read.SVG != string(drawing.Empty()) || read.ETag != created.ETag || rec.Header().Get("ETag") != `"`+created.ETag+`"` {
		t.Errorf("read: %+v %s", read, rec.Header().Get("ETag"))
	}

	// Saving needs If-Match and a drawing that passes the check.
	expect(t, admin.do("PUT", url, svgBody(savedDrawing)), 428, "if_match_required")
	expect(t, admin.do("PUT", url, svgBody(`<svg xmlns="http://www.w3.org/2000/svg"><metadata>{}</metadata><script>alert(1)</script></svg>`),
		"If-Match", `"`+created.ETag+`"`), 400, "invalid_drawing")
	rec = admin.do("PUT", url, svgBody(savedDrawing), "If-Match", `"`+created.ETag+`"`)
	expect(t, rec, 200, "")
	saved := decode[drawingFile](t, rec)
	if saved.ETag == created.ETag {
		t.Error("the ETag did not change")
	}
	expect(t, admin.do("PUT", url, svgBody(savedDrawing), "If-Match", `"`+created.ETag+`"`), 412, "")
	if got := decode[drawingFile](t, admin.do("GET", url, "")); got.SVG != savedDrawing {
		t.Errorf("after saving: %q", got.SVG)
	}

	// The file itself is an ordinary, sandboxed SVG.
	rec = admin.do("GET", base+"/files/Plans/Garden.drawing.svg", "")
	expect(t, rec, 200, "")
	if rec.Header().Get("Content-Type") != "image/svg+xml" || rec.Header().Get("Content-Security-Policy") != fileCSP {
		t.Errorf("served as %q with CSP %q", rec.Header().Get("Content-Type"), rec.Header().Get("Content-Security-Policy"))
	}

	// Viewers read, writers save, contributors create.
	member := fmt.Sprintf("%s/members/%d", base, benUser.ID)
	expect(t, admin.do("PUT", member, `{"role":"viewer"}`), 200, "")
	expect(t, ben.do("GET", url, ""), 200, "")
	expect(t, ben.do("PUT", url, svgBody(savedDrawing), "If-Match", `"`+saved.ETag+`"`), 403, "")
	expect(t, admin.do("PUT", member, `{"role":"writer"}`), 200, "")
	expect(t, ben.do("POST", base+"/drawings/Ben.drawing.svg", ""), 403, "")
	rec = ben.do("PUT", url, svgBody(strings.Replace(savedDrawing, `width="80"`, `width="90"`, 1)), "If-Match", `"`+saved.ETag+`"`)
	expect(t, rec, 200, "")
	expect(t, admin.do("PUT", member, `{"role":"contributor"}`), 200, "")
	expect(t, ben.do("POST", base+"/drawings/Ben.drawing.svg", ""), 201, "")
}

func TestIconLibrary(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	base := "/api/somas/" + testSoma
	icons := func(c *client) []store.Icon {
		return decode[struct{ Icons []store.Icon }](t, c.do("GET", base+"/icons", "")).Icons
	}

	// PNG is encoded anew, SVG is checked; anything else is refused.
	rec := admin.do("POST", base+"/icons?name=Tree", testPhoto(t, 64))
	expect(t, rec, 201, "")
	tree := decode[store.Icon](t, rec)
	if tree.Name != "Tree" || tree.Mime != "image/png" || tree.Size == 0 {
		t.Errorf("png icon: %+v", tree)
	}
	star := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M12 2l3 7h7l-6 4 2 8-6-5-6 5 2-8-6-4h7z"/></svg>`
	if icon := decode[store.Icon](t, admin.do("POST", base+"/icons?name=Star", star)); icon.Mime != "image/svg+xml" {
		t.Errorf("svg icon: %+v", icon)
	}
	expect(t, admin.do("POST", base+"/icons", "GIF89a"), 400, "invalid_icon")
	expect(t, admin.do("POST", base+"/icons", testPhoto(t, 600)), 400, "invalid_icon")
	expect(t, admin.do("POST", base+"/icons", strings.Replace(star, "<path", "<script>alert(1)</script><path", 1)), 400, "invalid_icon")
	expect(t, admin.do("POST", base+"/icons?name=a%0Ab", star), 400, "invalid_name")
	if list := icons(admin); len(list) != 2 || list[0].Name != "Star" || list[1].Name != "Tree" {
		t.Errorf("library: %+v", list)
	}

	// The picture is served sandboxed.
	rec = admin.do("GET", fmt.Sprintf("%s/icons/%d", base, tree.ID), "")
	expect(t, rec, 200, "")
	if rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("Content-Security-Policy") != fileCSP ||
		!strings.HasPrefix(rec.Body.String(), "\x89PNG") {
		t.Errorf("icon served as %q", rec.Header().Get("Content-Type"))
	}
	expect(t, admin.do("GET", base+"/icons/999", ""), 404, "not_found")
	expect(t, admin.do("GET", base+"/icons/x", ""), 404, "not_found")

	// Viewers use the library, contributors add to it; uploaders remove their
	// own icons, editors any.
	member := fmt.Sprintf("%s/members/%d", base, benUser.ID)
	expect(t, admin.do("PUT", member, `{"role":"viewer"}`), 200, "")
	if len(icons(ben)) != 2 {
		t.Error("a viewer does not see the library")
	}
	expect(t, ben.do("GET", fmt.Sprintf("%s/icons/%d", base, tree.ID), ""), 200, "")
	expect(t, ben.do("POST", base+"/icons", star), 403, "")
	expect(t, admin.do("PUT", member, `{"role":"contributor"}`), 200, "")
	rec = ben.do("POST", base+"/icons?name=Bench", star)
	expect(t, rec, 201, "")
	bench := decode[store.Icon](t, rec)
	expect(t, ben.do("DELETE", fmt.Sprintf("%s/icons/%d", base, tree.ID), ""), 403, "forbidden")
	expect(t, ben.do("DELETE", fmt.Sprintf("%s/icons/%d", base, bench.ID), ""), 200, "")
	expect(t, admin.do("PUT", member, `{"role":"editor"}`), 200, "")
	expect(t, ben.do("DELETE", fmt.Sprintf("%s/icons/%d", base, tree.ID), ""), 200, "")
	expect(t, ben.do("DELETE", fmt.Sprintf("%s/icons/%d", base, tree.ID), ""), 404, "not_found")

	// Icons belong to their soma.
	ctx := context.Background()
	if _, _, err := s.store.IconByID(ctx, "othersoma1", bench.ID+1); err == nil {
		t.Error("an icon of another soma was found")
	}

	// The library has a limit.
	for range store.MaxIconsPerSoma - len(icons(admin)) {
		if _, err := s.store.AddIcon(ctx, testSoma, "x", "image/svg+xml", []byte(star), 0); err != nil {
			t.Fatal(err)
		}
	}
	expect(t, admin.do("POST", base+"/icons", star), 409, "too_many_icons")
}
