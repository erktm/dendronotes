package server

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"dendronotes/internal/store"
)

func testPhoto(t *testing.T, size int) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, size, size))); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestAvatars(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	cara, _ := newUser(t, s, admin, "cara")
	me := func(c *client) store.User {
		return decode[struct{ User store.User }](t, c.do("GET", "/api/auth/me", "")).User
	}
	photoURL := func(u store.User) string { return fmt.Sprintf("/api/users/%d/avatar", u.ID) }

	// The design: color, pattern, initials.
	rec := ben.do("PUT", "/api/me/avatar", `{"color":3,"pattern":"dendrite","initials":"bn"}`)
	expect(t, rec, 200, "")
	if a := decode[store.User](t, rec).Avatar; a != (store.Avatar{Color: 3, Pattern: "dendrite", Initials: "BN"}) {
		t.Errorf("design: %+v", a)
	}
	expect(t, ben.do("PUT", "/api/me/avatar", `{"color":3,"pattern":"spiral","initials":""}`), 400, "invalid_avatar")
	expect(t, ben.do("PUT", "/api/me/avatar", `{"color":9,"pattern":"plain","initials":""}`), 400, "invalid_avatar")
	expect(t, ben.do("PUT", "/api/me/avatar", `{"color":1,"pattern":"plain","photo":5}`), 400, "")
	if a := me(ben).Avatar; a.Pattern != "dendrite" {
		t.Errorf("refused designs changed the avatar: %+v", a)
	}

	// Photos: only square PNGs, encoded anew.
	upload := func(c *client, body, contentType string) *httptest.ResponseRecorder {
		return c.do("PUT", "/api/me/avatar/photo", body, "Content-Type", contentType)
	}
	expect(t, upload(ben, testPhoto(t, 128), "image/jpeg"), 415, "unsupported_type")
	expect(t, upload(ben, "<svg onload=alert(1)>", "image/png"), 400, "invalid_photo")
	expect(t, upload(ben, testPhoto(t, 1024), "image/png"), 400, "invalid_photo")
	expect(t, upload(ben, strings.Repeat("x", 2<<20), "image/png"), 413, "too_large")
	rec = upload(ben, testPhoto(t, 128), "image/png")
	expect(t, rec, 200, "")
	version := decode[store.User](t, rec).Avatar.Photo
	if version == 0 || me(ben).Avatar.Photo != version {
		t.Fatalf("photo version: %d", version)
	}

	// The photo itself.
	rec = ben.do("GET", photoURL(benUser)+"?v="+strconv.FormatInt(version, 10), "")
	expect(t, rec, 200, "")
	if rec.Header().Get("Content-Type") != "image/png" || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("headers: %v", rec.Header())
	}
	if img, err := png.Decode(rec.Body); err != nil || img.Bounds().Dx() != 128 {
		t.Errorf("photo: %v", err)
	}
	if cc := ben.do("GET", photoURL(benUser)+"?v=1", "").Header().Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("an old version must not be cached for good: %q", cc)
	}
	etag := `"` + strconv.FormatInt(version, 10) + `"`
	expect(t, ben.do("GET", photoURL(benUser), "", "If-None-Match", etag), 304, "")

	// Others see it only when they share a soma; admins always.
	expect(t, cara.do("GET", photoURL(benUser), ""), 404, "not_found")
	expect(t, admin.do("GET", photoURL(benUser), ""), 200, "")
	expect(t, ben.do("GET", "/api/users/abc/avatar", ""), 404, "not_found")
	adminUser := me(admin)
	expect(t, upload(admin, testPhoto(t, 64), "image/png"), 200, "")
	expect(t, cara.do("GET", photoURL(adminUser), ""), 404, "not_found")
	expect(t, admin.do("PUT", fmt.Sprintf("/api/somas/%s/members/%d", testSoma, me(cara).ID), `{"role":"viewer"}`), 200, "")
	expect(t, cara.do("GET", photoURL(adminUser), ""), 200, "")
	members := decode[struct{ Members []store.Member }](t, cara.do("GET", "/api/somas/"+testSoma+"/members", "")).Members
	if len(members) != 2 || members[0].Avatar == nil || members[0].Avatar.Photo == 0 {
		t.Errorf("members carry avatars: %+v", members)
	}

	// Removing the photo brings the design back.
	rec = ben.do("DELETE", "/api/me/avatar/photo", "")
	expect(t, rec, 200, "")
	if a := decode[store.User](t, rec).Avatar; a.Photo != 0 || a.Pattern != "dendrite" {
		t.Errorf("after removing: %+v", a)
	}
	expect(t, ben.do("GET", photoURL(benUser), ""), 404, "not_found")
}
