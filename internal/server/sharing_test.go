package server

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/store"
)

// newUser creates an account through the admin API and signs it in.
func newUser(t *testing.T, s *Server, admin *client, name string) (*client, store.User) {
	t.Helper()
	rec := admin.do("POST", "/api/admin/users", fmt.Sprintf(`{"username":%q,"password":"%s password long","role":"user"}`, name, name))
	expect(t, rec, 201, "")
	c := newClient(t, s)
	c.loginAs(name, name+" password long")
	return c, decode[store.User](t, rec)
}

func TestSharing(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	cara, caraUser := newUser(t, s, admin, "cara")
	_, dave := newUser(t, s, admin, "dave")
	expect(t, admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", dave.ID), `{"disabled":true}`), 200, "")

	base := "/api/somas/" + testSoma
	member := func(u store.User) string { return fmt.Sprintf("%s/members/%d", base, u.ID) }
	expect(t, admin.do("POST", base+"/notes/Welcome.md", `{"content":"Hi"}`), 201, "")

	// Finding people needs two characters and shows only names.
	found := decode[struct{ Users []map[string]any }](t, admin.do("GET", "/api/users/search?q=be", "")).Users
	if len(found) != 1 || found[0]["username"] != "ben" || len(found[0]) != 3 {
		t.Errorf("search: %v", found)
	}
	if found := decode[struct{ Users []store.Person }](t, admin.do("GET", "/api/users/search?q=b", "")).Users; len(found) != 0 {
		t.Errorf("one-letter search: %v", found)
	}
	if found := decode[struct{ Users []store.Person }](t, admin.do("GET", "/api/users/search?q=da", "")).Users; len(found) != 0 {
		t.Errorf("disabled accounts are found: %v", found)
	}

	// Sharing tells connected clients.
	events, cancel, _ := rtOf(t, s).Hub.Subscribe()
	defer cancel()
	expect(t, admin.do("PUT", member(benUser), `{"role":"viewer"}`), 200, "")
	select {
	case e := <-events:
		if e.Name != "access" {
			t.Errorf("event %q, want access", e.Name)
		}
	case <-time.After(2 * time.Second):
		t.Error("no access event")
	}
	expect(t, admin.do("PUT", member(benUser), `{"role":"owner"}`), 400, "invalid_role")
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/1", base), `{"role":"editor"}`), 400, "self")
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/999", base), `{"role":"editor"}`), 400, "unknown_user")
	expect(t, admin.do("PUT", member(dave), `{"role":"editor"}`), 400, "unknown_user")

	// A viewer reads but changes nothing.
	list := decode[struct{ Somas []store.Membership }](t, ben.do("GET", "/api/somas", "")).Somas
	var shared *store.Membership
	for i := range list {
		if list[i].ID == testSoma {
			shared = &list[i]
		}
	}
	if shared == nil || shared.Role != store.MemberViewer || shared.Owner.Username != "admin" {
		t.Fatalf("ben's somas: %+v", list)
	}
	expect(t, ben.do("GET", base+"/notes/Welcome.md", ""), 200, "")
	expect(t, ben.do("POST", base+"/notes/Mine.md", `{"content":"x"}`), 403, "forbidden")
	expect(t, ben.do("PUT", member(caraUser), `{"role":"viewer"}`), 403, "forbidden")
	members := decode[struct {
		Members []store.Member
		Role    string
	}](t, ben.do("GET", base+"/members", ""))
	if members.Role != store.MemberViewer || len(members.Members) != 2 || members.Members[0].Role != store.MemberOwner {
		t.Errorf("members: %+v", members)
	}

	// As editor, ben writes notes.
	expect(t, admin.do("PUT", member(benUser), `{"role":"editor"}`), 200, "")
	expect(t, ben.do("POST", base+"/notes/Mine.md", `{"content":"x"}`), 201, "")

	// Ownership goes only to members; the old owner stays as editor.
	expect(t, admin.do("PATCH", base, `{"ownerId":`+fmt.Sprint(caraUser.ID)+`}`), 400, "invalid_transfer")
	rec := admin.do("PATCH", base, `{"ownerId":`+fmt.Sprint(benUser.ID)+`}`)
	expect(t, rec, 200, "")
	if m := decode[store.Membership](t, rec); m.Role != store.MemberEditor || m.Owner.Username != "ben" {
		t.Errorf("after transfer: %+v", m)
	}
	expect(t, admin.do("DELETE", base, ""), 403, "forbidden")
	expect(t, ben.do("DELETE", base+"/membership", ""), 400, "owner_cannot_leave")

	// The new owner shares further and takes access away.
	expect(t, ben.do("PUT", member(caraUser), `{"role":"viewer"}`), 200, "")
	expect(t, ben.do("DELETE", fmt.Sprintf("%s/members/1", base), ""), 200, "")
	expect(t, admin.do("GET", base+"/tree", ""), 404, "not_found")

	// Anyone may leave a soma shared with them.
	expect(t, cara.do("DELETE", base+"/membership", ""), 200, "")
	expect(t, cara.do("GET", base+"/tree", ""), 404, "not_found")

	var actions []string
	for _, e := range decode[struct{ Entries []store.AuditEntry }](t, admin.do("GET", "/api/admin/audit", "")).Entries {
		if strings.HasPrefix(e.Action, "member.") || e.Action == "soma.transfer" {
			actions = append(actions, e.Action)
		}
	}
	if got := strings.Join(actions, ","); got != "member.leave,member.remove,member.set,soma.transfer,member.set,member.set" {
		t.Errorf("audit: %s", got)
	}
}
