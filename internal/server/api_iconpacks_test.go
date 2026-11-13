package server

import (
	"archive/zip"
	"bytes"
	"strconv"
	"strings"
	"testing"

	"dendronotes/internal/store"
)

const packSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M4 4h16v16H4z"/></svg>`

func iconPack(t *testing.T, files ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		content := packSVG
		if strings.HasSuffix(name, ".txt") {
			content = "notes"
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestIconPacksAndSections(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	base := "/api/somas/" + testSoma

	type library struct {
		Icons    []store.Icon        `json:"icons"`
		Sections []store.IconSection `json:"sections"`
		Server   struct {
			Icons    []store.Icon        `json:"icons"`
			Sections []store.IconSection `json:"sections"`
		} `json:"server"`
	}
	type packResult struct {
		Added    int      `json:"added"`
		Sections []string `json:"sections"`
		Skipped  []struct {
			File string `json:"file"`
		} `json:"skipped"`
	}

	// A pack into the soma: folders become sections, the rest one named after the pack.
	rec := admin.upload(base+"/icons/pack?name=Office.zip", string(iconPack(t, "Office/Cloud/server.svg", "Office/Cloud/db.svg", "Office/printer.svg", "Office/readme.txt")))
	expect(t, rec, 201, "")
	got := decode[packResult](t, rec)
	if got.Added != 3 || strings.Join(got.Sections, ",") != "Cloud,Office" || len(got.Skipped) != 1 {
		t.Fatalf("pack: %+v", got)
	}
	expect(t, admin.upload(base+"/icons/pack?name=x.zip", "not a zip"), 400, "invalid_pack")
	lib := decode[library](t, admin.do("GET", base+"/icons", ""))
	if len(lib.Icons) != 3 || len(lib.Sections) != 2 {
		t.Fatalf("library: %+v", lib)
	}
	cloud := lib.Sections[0]
	for _, i := range lib.Icons {
		if i.SectionID == 0 {
			t.Errorf("icon without section: %+v", i)
		}
	}

	// A second pack with the same folder adds to the section.
	expect(t, admin.upload(base+"/icons/pack?name=More.zip", string(iconPack(t, "Cloud/lambda.svg"))), 201, "")
	lib = decode[library](t, admin.do("GET", base+"/icons", ""))
	if len(lib.Sections) != 2 || len(lib.Icons) != 4 {
		t.Fatalf("after the second pack: %+v", lib)
	}

	// Sections: rename, order, move an icon out, delete with the icons in it.
	expect(t, admin.do("PATCH", base+"/icon-sections/"+itoa(cloud.ID), `{"name":"Cloud services"}`), 204, "")
	expect(t, admin.do("PUT", base+"/icon-sections/order", `{"ids":[`+itoa(lib.Sections[1].ID)+`]}`), 204, "")
	lib = decode[library](t, admin.do("GET", base+"/icons", ""))
	if lib.Sections[0].Name != "Office" || lib.Sections[1].Name != "Cloud services" {
		t.Fatalf("order: %+v", lib.Sections)
	}
	var one store.Icon
	for _, i := range lib.Icons {
		if i.SectionID == cloud.ID {
			one = i
			break
		}
	}
	expect(t, admin.do("PATCH", base+"/icons/"+itoa(one.ID), `{"sectionId":0}`), 200, "")
	expect(t, admin.do("PATCH", base+"/icons/"+itoa(one.ID), `{"sectionId":999}`), 404, "not_found")
	expect(t, admin.do("DELETE", base+"/icon-sections/"+itoa(cloud.ID), ""), 200, "")
	lib = decode[library](t, admin.do("GET", base+"/icons", ""))
	if len(lib.Sections) != 1 || len(lib.Icons) != 2 {
		t.Fatalf("after deleting a section: %+v", lib)
	}

	// The server's library: admins fill it, everyone signed in sees and loads it.
	expect(t, admin.upload("/api/admin/icons/pack?name=Shared.zip", string(iconPack(t, "logo.svg"))), 201, "")
	ben, _ := newUser(t, s, admin, "ben")
	expect(t, ben.upload("/api/admin/icons/pack?name=Shared.zip", string(iconPack(t, "logo.svg"))), 403, "forbidden")
	lib = decode[library](t, admin.do("GET", base+"/icons", ""))
	if len(lib.Server.Icons) != 1 || len(lib.Server.Sections) != 1 || lib.Server.Sections[0].Name != "Shared" {
		t.Fatalf("server library: %+v", lib.Server)
	}
	expect(t, ben.do("GET", "/api/icons/"+itoa(lib.Server.Icons[0].ID), ""), 200, "")
	expect(t, newClient(t, s).do("GET", "/api/icons/"+itoa(lib.Server.Icons[0].ID), ""), 401, "unauthorized")
	// A soma's icons are not reachable as server icons.
	expect(t, ben.do("GET", "/api/icons/"+itoa(lib.Icons[0].ID), ""), 404, "not_found")
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
