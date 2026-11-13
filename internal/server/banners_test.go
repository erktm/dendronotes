package server

import (
	"fmt"
	"testing"
	"time"

	"dendronotes/internal/store"
)

type bannerList struct {
	Banners []struct {
		ID          int64
		Message     string
		Kind        string
		Dismissible bool
		Revision    int64
	}
}

func TestBannersAPI(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	anon := newClient(t, s)

	// Checks.
	for body, code := range map[string]string{
		`{"message":"","kind":"info","audience":"everyone","inApp":true}`:                                                                     "invalid_banner_text",
		`{"message":"Hi","kind":"loud","audience":"everyone","inApp":true}`:                                                                   "invalid_banner_kind",
		`{"message":"Hi","kind":"info","audience":"nobody","inApp":true}`:                                                                     "invalid_banner_audience",
		`{"message":"Hi","kind":"info","audience":"group","groupId":999,"inApp":true}`:                                                        "unknown_group",
		`{"message":"Hi","kind":"info","audience":"everyone"}`:                                                                                "banner_nowhere",
		`{"message":"Hi","kind":"info","audience":"admins","onLogin":true}`:                                                                   "banner_login_audience",
		`{"message":"Hi","kind":"info","audience":"everyone","inApp":true,"startsAt":"2030-01-02T00:00:00Z","endsAt":"2030-01-01T00:00:00Z"}`: "banner_ends_before_start",
	} {
		expect(t, admin.do("POST", "/api/admin/banners", body), 400, code)
	}
	expect(t, ben.do("POST", "/api/admin/banners", `{"message":"Hi","kind":"info","audience":"everyone","inApp":true}`), 403, "")

	create := func(body string) store.Banner {
		t.Helper()
		rec := admin.do("POST", "/api/admin/banners", body)
		expect(t, rec, 201, "")
		return decode[store.Banner](t, rec)
	}
	login := create(`{"message":"**Update** tonight","kind":"warning","audience":"everyone","onLogin":true,"inApp":true,"dismissible":true}`)
	fixed := create(`{"message":"Admins: check the backups","kind":"critical","audience":"admins","inApp":true}`)
	soon := create(fmt.Sprintf(`{"message":"Soon","kind":"info","audience":"everyone","inApp":true,"startsAt":%q}`,
		time.Now().Add(time.Hour).UTC().Format(time.RFC3339)))
	_ = soon

	// The sign-in page and the app.
	got := decode[bannerList](t, anon.do("GET", "/api/auth/banners", ""))
	if len(got.Banners) != 1 || got.Banners[0].ID != login.ID || got.Banners[0].Message != "**Update** tonight" {
		t.Errorf("sign-in page: %+v", got)
	}
	expect(t, anon.do("GET", "/api/me/banners", ""), 401, "")
	if got := decode[bannerList](t, ben.do("GET", "/api/me/banners", "")); len(got.Banners) != 1 || got.Banners[0].ID != login.ID {
		t.Errorf("ben: %+v", got)
	}
	if got := decode[bannerList](t, admin.do("GET", "/api/me/banners", "")); len(got.Banners) != 2 || got.Banners[0].ID != fixed.ID {
		t.Errorf("admin (critical first): %+v", got)
	}

	// Closing: not with tokens, not the fixed ones; back when the text changes.
	tok := decode[struct{ Token string }](t, ben.do("POST", "/api/me/tokens", `{"name":"script","scope":"write"}`)).Token
	expect(t, anon.do("POST", fmt.Sprintf("/api/me/banners/%d/dismiss", login.ID), "", "Authorization", "Bearer "+tok), 403, "token_not_allowed")
	expect(t, admin.do("POST", fmt.Sprintf("/api/me/banners/%d/dismiss", fixed.ID), ""), 400, "not_dismissible")
	expect(t, ben.do("POST", "/api/me/banners/999/dismiss", ""), 404, "not_found")
	expect(t, ben.do("POST", fmt.Sprintf("/api/me/banners/%d/dismiss", login.ID), ""), 200, "")
	if got := decode[bannerList](t, ben.do("GET", "/api/me/banners", "")); len(got.Banners) != 0 {
		t.Errorf("closed: %+v", got)
	}
	edit := fmt.Sprintf("/api/admin/banners/%d", login.ID)
	rec := admin.do("PATCH", edit, `{"message":"**Update** moved to Friday","kind":"warning","audience":"everyone","onLogin":true,"inApp":true,"dismissible":true}`)
	expect(t, rec, 200, "")
	if b := decode[store.Banner](t, rec); b.Revision != 2 {
		t.Errorf("revision %d", b.Revision)
	}
	if got := decode[bannerList](t, ben.do("GET", "/api/me/banners", "")); len(got.Banners) != 1 {
		t.Errorf("new text: %+v", got)
	}

	// A group audience; the group Everyone means everyone.
	g := decode[store.Group](t, admin.do("POST", "/api/admin/groups", `{"name":"Garden"}`))
	expect(t, admin.do("PUT", fmt.Sprintf("/api/admin/groups/%d/members/%d", g.ID, benUser.ID), ""), 200, "")
	garden := create(fmt.Sprintf(`{"message":"Garden","kind":"info","audience":"group","groupId":%d,"inApp":true}`, g.ID))
	if garden.GroupName != "Garden" {
		t.Errorf("group: %+v", garden)
	}
	if got := decode[bannerList](t, admin.do("GET", "/api/me/banners", "")); len(got.Banners) != 2 {
		t.Errorf("admin sees the group banner: %+v", got)
	}
	var everyone store.Group
	for _, gr := range decode[struct{ Groups []store.Group }](t, admin.do("GET", "/api/admin/groups", "")).Groups {
		if gr.Everyone {
			everyone = gr
		}
	}
	all := create(fmt.Sprintf(`{"message":"All","kind":"info","audience":"group","groupId":%d,"inApp":true,"onLogin":true}`, everyone.ID))
	if all.Audience != "everyone" || all.GroupID != 0 {
		t.Errorf("everyone group: %+v", all)
	}

	// Deleting; the list is for admins only.
	expect(t, admin.do("DELETE", edit, ""), 200, "")
	expect(t, admin.do("DELETE", edit, ""), 404, "not_found")
	expect(t, ben.do("GET", "/api/admin/banners", ""), 403, "")
	if list := decode[struct{ Banners []store.Banner }](t, admin.do("GET", "/api/admin/banners", "")).Banners; len(list) != 4 {
		t.Errorf("left: %d", len(list))
	}
}
