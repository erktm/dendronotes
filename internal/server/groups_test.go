package server

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/store"
)

// What each role may do with the files of a soma, end to end.
func TestRolesInPractice(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	base := "/api/somas/" + testSoma
	member := fmt.Sprintf("%s/members/%d", base, benUser.ID)

	rec := admin.do("POST", base+"/notes/Plan.md", `{"content":"beds"}`)
	expect(t, rec, 201, "")
	etag := decode[map[string]any](t, rec)["etag"].(string)
	save := func(c *client) *httptest.ResponseRecorder {
		rec := c.do("PUT", base+"/notes/Plan.md", `{"content":"beds and paths"}`, "If-Match", `"`+etag+`"`)
		if rec.Code == 200 {
			etag = decode[map[string]any](t, rec)["etag"].(string)
		}
		return rec
	}

	// writer: change notes, nothing else.
	expect(t, admin.do("PUT", member, `{"role":"writer"}`), 200, "")
	expect(t, save(ben), 200, "")
	expect(t, ben.do("POST", base+"/notes/New.md", `{"content":""}`), 403, "forbidden")
	expect(t, ben.do("POST", base+"/folders/Seeds", ``), 403, "forbidden")
	expect(t, ben.do("DELETE", base+"/entries/Plan.md", ``), 403, "forbidden")

	// contributor: also create, but not rename, move or delete.
	expect(t, admin.do("PUT", member, `{"role":"contributor"}`), 200, "")
	expect(t, ben.do("POST", base+"/notes/New.md", `{"content":""}`), 201, "")
	expect(t, ben.do("POST", base+"/folders/Seeds", ``), 201, "")
	expect(t, ben.do("POST", base+"/move", `{"from":"New.md","to":"Seeds/New.md"}`), 403, "forbidden")
	expect(t, ben.do("DELETE", base+"/entries/New.md", ``), 403, "forbidden")

	// editor: everything with files, nothing with sharing.
	expect(t, admin.do("PUT", member, `{"role":"editor"}`), 200, "")
	expect(t, ben.do("POST", base+"/move", `{"from":"New.md","to":"Seeds/New.md"}`), 200, "")
	expect(t, ben.do("DELETE", base+"/entries/Seeds", ``), 200, "")
	expect(t, ben.do("PUT", member, `{"role":"viewer"}`), 403, "forbidden")

	expect(t, admin.do("PUT", member, `{"role":"owner"}`), 400, "invalid_role")
	expect(t, admin.do("PUT", member, `{"role":"admin"}`), 400, "invalid_role")
}

func TestGroups(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	cara, caraUser := newUser(t, s, admin, "cara")
	dave, _ := newUser(t, s, admin, "dave")
	base := "/api/somas/" + testSoma

	// Groups are managed by admins; Everyone is built in.
	list := decode[struct{ Groups []store.Group }](t, admin.do("GET", "/api/admin/groups", "")).Groups
	if len(list) != 1 || !list[0].Everyone || list[0].Members != 3 {
		t.Fatalf("groups at the start: %+v", list)
	}
	everyone := fmt.Sprintf("/api/admin/groups/%d", list[0].ID)
	expect(t, admin.do("PATCH", everyone, `{"name":"All"}`), 400, "builtin_group")
	expect(t, admin.do("DELETE", everyone, ``), 400, "builtin_group")
	expect(t, admin.do("GET", everyone+"/members", ``), 400, "builtin_group")
	expect(t, admin.do("POST", "/api/admin/groups", `{"name":" "}`), 400, "invalid_name")
	expect(t, admin.do("POST", "/api/admin/groups", `{"name":"everyone"}`), 409, "name_taken")
	expect(t, cara.do("POST", "/api/admin/groups", `{"name":"Mine"}`), 403, "")

	rec := admin.do("POST", "/api/admin/groups", `{"name":"Garden club"}`)
	expect(t, rec, 201, "")
	club := decode[store.Group](t, rec)
	clubURL := fmt.Sprintf("/api/admin/groups/%d", club.ID)
	rec = admin.do("PUT", fmt.Sprintf("%s/members/%d", clubURL, caraUser.ID), ``)
	expect(t, rec, 200, "")
	if m := decode[struct{ Members []store.Person }](t, rec).Members; len(m) != 1 || m[0].Username != "cara" {
		t.Fatalf("club members: %+v", m)
	}
	expect(t, admin.do("PUT", clubURL+"/members/9999", ``), 400, "unknown_user")
	expect(t, admin.do("PATCH", clubURL, `{"name":"Allotment"}`), 200, "")

	// Owners share with groups; the search finds them.
	found := decode[struct {
		Users  []store.Person
		Groups []store.Group
	}](t, admin.do("GET", "/api/users/search?q=al", ""))
	if len(found.Groups) != 1 || found.Groups[0].Name != "Allotment" {
		t.Fatalf("search: %+v", found)
	}
	somaGroup := fmt.Sprintf("%s/groups/%d", base, club.ID)
	expect(t, cara.do("GET", base+"/tree", ""), 404, "not_found")
	expect(t, admin.do("PUT", somaGroup, `{"role":"owner"}`), 400, "invalid_role")
	expect(t, admin.do("PUT", base+"/groups/9999", `{"role":"viewer"}`), 404, "unknown_group")
	rec = admin.do("PUT", somaGroup, `{"role":"contributor"}`)
	expect(t, rec, 200, "")
	if g := decode[struct{ Groups []store.GroupGrant }](t, rec).Groups; len(g) != 1 || g[0].Role != "contributor" {
		t.Fatalf("grants: %+v", g)
	}
	expect(t, cara.do("GET", base+"/tree", ""), 200, "")
	expect(t, cara.do("POST", base+"/notes/Cara.md", `{"content":""}`), 201, "")
	expect(t, cara.do("DELETE", base+"/entries/Cara.md", ``), 403, "forbidden")
	var shared *store.Membership
	for _, m := range decode[struct{ Somas []store.Membership }](t, cara.do("GET", "/api/somas", "")).Somas {
		if m.ID == testSoma {
			shared = &m
		}
	}
	if shared == nil || shared.Role != "contributor" || shared.Direct {
		t.Fatalf("cara's view of the shared soma: %+v", shared)
	}
	expect(t, cara.do("DELETE", base+"/membership", ``), 400, "not_shared_personally")
	expect(t, cara.do("PUT", somaGroup, `{"role":"editor"}`), 403, "forbidden")

	// Shared with everyone: every account, also dave who is in no group.
	expect(t, dave.do("GET", base+"/tree", ""), 404, "not_found")
	expect(t, admin.do("PUT", fmt.Sprintf("%s/groups/%d", base, list[0].ID), `{"role":"viewer"}`), 200, "")
	expect(t, dave.do("GET", base+"/tree", ""), 200, "")
	expect(t, dave.do("POST", base+"/notes/Dave.md", `{"content":""}`), 403, "forbidden")
	expect(t, admin.do("DELETE", fmt.Sprintf("%s/groups/%d", base, list[0].ID), ``), 200, "")
	expect(t, dave.do("GET", base+"/tree", ""), 404, "not_found")

	// Leaving the group ends cara's live stream at once.
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+base+"/events", nil)
	req.AddCookie(cara.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := bufio.NewScanner(resp.Body)
	for lines.Scan() && lines.Text() != "event: hello" {
	}
	ended := make(chan struct{})
	go func() {
		for lines.Scan() {
		}
		close(ended)
	}()
	expect(t, admin.do("DELETE", fmt.Sprintf("%s/members/%d", clubURL, caraUser.ID), ``), 200, "")
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream outlived the access")
	}
	expect(t, cara.do("GET", base+"/tree", ""), 404, "not_found")

	// Deleting a group takes its shares along.
	expect(t, admin.do("DELETE", clubURL, ``), 200, "")
	if g := decode[struct{ Groups []store.GroupGrant }](t, admin.do("GET", base+"/members", "")).Groups; len(g) != 0 {
		t.Errorf("grants of a deleted group: %+v", g)
	}
	if !strings.Contains(admin.do("GET", "/api/admin/audit", "").Body.String(), "group.delete") {
		t.Error("group changes are audited")
	}
}
