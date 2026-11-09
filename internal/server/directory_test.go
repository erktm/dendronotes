package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"dendronotes/internal/store"
)

// Accounts and groups from Active Directory: name, e-mail and members come
// from the directory, so the API does not change them.
func TestDirectoryAccountsAndGroupsReadOnly(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ctx := context.Background()

	ben, err := s.store.CreateUser(ctx, store.User{Username: "ben", DisplayName: "Ben Miller", Email: "ben@corp.example",
		Role: store.RoleUser, Source: store.SourceLDAP, LDAPID: "guid-ben"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.SyncDirectoryGroups(ctx, ben.ID, []store.DirectoryGroup{{ID: "guid-garden", Name: "Garden"}}); err != nil {
		t.Fatal(err)
	}
	token, err := s.store.CreateSession(ctx, ben.ID, time.Hour, "", "")
	if err != nil {
		t.Fatal(err)
	}
	benClient := newClient(t, s)
	benClient.cookie = &http.Cookie{Name: admin.cookie.Name, Value: token}

	// The profile follows the directory.
	expect(t, benClient.do("PATCH", "/api/me", `{"displayName":"Other"}`), 400, "managed_by_provider")
	expect(t, benClient.do("POST", "/api/me/password", `{"current":"x","new":"long enough pw"}`), 400, "not_local")

	// The group is listed as a directory group; it can be shared and deleted,
	// but neither renamed nor given other members.
	var garden store.Group
	for _, g := range decode[struct{ Groups []store.Group }](t, admin.do("GET", "/api/admin/groups", "")).Groups {
		if g.Name == "Garden" {
			garden = g
		}
	}
	if garden.Source != "ldap" || garden.Members != 1 {
		t.Fatalf("directory group: %+v", garden)
	}
	url := fmt.Sprintf("/api/admin/groups/%d", garden.ID)
	expect(t, admin.do("PATCH", url, `{"name":"Allotment"}`), 400, "directory_group")
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/1", url), ``), 400, "directory_group")
	expect(t, admin.do("DELETE", fmt.Sprintf("%s/members/%d", url, ben.ID), ``), 400, "directory_group")
	expect(t, admin.do("GET", url+"/members", ``), 200, "")

	base := "/api/somas/" + testSoma
	expect(t, benClient.do("GET", base+"/tree", ""), 404, "not_found")
	expect(t, admin.do("PUT", fmt.Sprintf("%s/groups/%d", base, garden.ID), `{"role":"viewer"}`), 200, "")
	expect(t, benClient.do("GET", base+"/tree", ""), 200, "")
	expect(t, admin.do("DELETE", url, ``), 200, "")
	expect(t, benClient.do("GET", base+"/tree", ""), 404, "not_found")

	// Without LDAP there is nothing to check.
	expect(t, admin.do("POST", "/api/admin/ldap/check", ``), 404, "not_available")
	if info := decode[map[string]any](t, admin.do("GET", "/api/admin/server", "")); info["ldap"] != nil {
		t.Errorf("ldap status without LDAP: %v", info["ldap"])
	}
}
