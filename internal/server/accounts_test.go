package server

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"dendronotes/internal/store"
)

func TestSomaLifecycle(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	c.login()

	rec := c.do("POST", "/api/somas", `{"name":"Garden Club"}`)
	expect(t, rec, 201, "")
	garden := decode[store.Membership](t, rec)
	if garden.Role != store.MemberOwner || garden.Folder != "garden-club" || garden.Owner.Username != "admin" {
		t.Fatalf("created: %+v", garden)
	}
	expect(t, c.do("POST", "/api/somas", `{"name":"  "}`), 400, "invalid_name")
	expect(t, c.do("POST", "/api/somas/"+garden.ID+"/notes/Hello.md", `{"content":"Hi"}`), 201, "")

	list := decode[struct{ Somas []store.Membership }](t, c.do("GET", "/api/somas", "")).Somas
	var names []string
	for _, m := range list {
		names = append(names, m.Name)
	}
	if strings.Join(names, ",") != "Garden Club,Notes,Test" {
		t.Errorf("somas: %v", names)
	}

	expect(t, c.do("PATCH", "/api/somas/"+garden.ID, `{"name":"Garden"}`), 200, "")
	if sm, _ := s.store.SomaByID(context.Background(), garden.ID); sm.Name != "Garden" || sm.Folder != "garden-club" {
		t.Errorf("renamed: %+v", sm)
	}
	expect(t, c.do("DELETE", "/api/somas/"+garden.ID, ""), 200, "")
	expect(t, c.do("GET", "/api/somas/"+garden.ID+"/tree", ""), 404, "not_found")
	trashed, _ := filepath.Glob(filepath.Join(s.cfg.SomasDir, ".trash", "garden-club-*", "Hello.md"))
	if len(trashed) != 1 {
		t.Errorf("deleted soma not in the trash: %v", trashed)
	}
}

func TestProfileAndPassword(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	c.login()
	other := newClient(t, s)
	other.login()

	rec := c.do("PATCH", "/api/me", `{"displayName":" Erik ","email":"erik@example.org"}`)
	expect(t, rec, 200, "")
	if u := decode[store.User](t, rec); u.DisplayName != "Erik" || u.Email != "erik@example.org" {
		t.Errorf("profile: %+v", u)
	}
	expect(t, c.do("PATCH", "/api/me", `{"email":"not an address"}`), 400, "invalid_email")

	expect(t, c.do("POST", "/api/me/password", `{"current":"wrong","new":"a new password"}`), 403, "wrong_password")
	expect(t, c.do("POST", "/api/me/password", `{"current":"`+testPassword+`","new":"short"}`), 400, "weak_password")
	expect(t, c.do("POST", "/api/me/password", `{"current":"`+testPassword+`","new":"a new password"}`), 200, "")
	// This session stays, the other one ends.
	expect(t, c.do("GET", "/api/somas", ""), 200, "")
	expect(t, other.do("GET", "/api/somas", ""), 401, "unauthorized")
	fresh := newClient(t, s)
	fresh.loginAs("admin", "a new password")
}

func TestAdminUsers(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	c.login()

	rec := c.do("POST", "/api/admin/users", `{"username":"anna","displayName":"Anna","password":"anna password long","role":"user"}`)
	expect(t, rec, 201, "")
	anna := decode[store.User](t, rec)
	expect(t, c.do("POST", "/api/admin/users", `{"username":"ANNA","password":"anna password long","role":"user"}`), 409, "username_taken")
	expect(t, c.do("POST", "/api/admin/users", `{"username":"x y","password":"anna password long","role":"user"}`), 400, "invalid_username")
	expect(t, c.do("POST", "/api/admin/users", `{"username":"bob","password":"short","role":"user"}`), 400, "weak_password")
	expect(t, c.do("POST", "/api/admin/users", `{"username":"bob","password":"bob password long","role":"root"}`), 400, "invalid_role")

	// A new account starts with its own soma.
	a := newClient(t, s)
	a.loginAs("anna", "anna password long")
	mine := decode[struct{ Somas []store.Membership }](t, a.do("GET", "/api/somas", "")).Somas
	if len(mine) != 1 || mine[0].Name != "Notes" {
		t.Fatalf("anna's somas: %+v", mine)
	}

	id := strconv.FormatInt(anna.ID, 10)
	expect(t, c.do("POST", "/api/admin/users/"+id+"/password", `{"password":"reset password long"}`), 200, "")
	expect(t, a.do("GET", "/api/somas", ""), 401, "unauthorized") // sessions ended
	a.loginAs("anna", "reset password long")
	expect(t, c.do("PATCH", "/api/admin/users/"+id, `{"disabled":true}`), 200, "")
	expect(t, a.do("GET", "/api/somas", ""), 401, "unauthorized")

	// The last admin stays.
	admin, _ := s.store.UserByUsername(context.Background(), "admin")
	aid := strconv.FormatInt(admin.ID, 10)
	expect(t, c.do("PATCH", "/api/admin/users/"+aid, `{"role":"user"}`), 409, "last_admin")
	expect(t, c.do("PATCH", "/api/admin/users/"+aid, `{"disabled":true}`), 409, "last_admin")
	expect(t, c.do("DELETE", "/api/admin/users/"+aid, ""), 409, "self")

	// Deleting hands the somas over.
	expect(t, c.do("DELETE", "/api/admin/users/"+id+"?transferTo="+aid, ""), 200, "")
	if sm, _ := s.store.SomaByID(context.Background(), mine[0].ID); sm.OwnerID != admin.ID {
		t.Errorf("soma not transferred: %+v", sm)
	}
	expect(t, c.do("DELETE", "/api/admin/users/"+id, ""), 404, "not_found")
}

func TestAdminSomasSessionsServerAudit(t *testing.T) {
	s := newTestServer(t, testFS())
	c := newClient(t, s)
	c.login()
	os.WriteFile(filepath.Join(somaDir(s), "a.md"), []byte("hello"), 0o644)

	somas := decode[struct{ Somas []adminSoma }](t, c.do("GET", "/api/admin/somas", "")).Somas
	var test adminSoma
	for _, sm := range somas {
		if sm.ID == testSoma {
			test = sm
		}
	}
	if test.Files != 1 || test.Bytes != 5 || test.Owner.Username != "admin" {
		t.Errorf("test soma: %+v", test)
	}

	sessions := decode[struct {
		Sessions []store.Session
		Current  int64
	}](t, c.do("GET", "/api/admin/sessions", ""))
	if len(sessions.Sessions) != 1 || sessions.Current != sessions.Sessions[0].ID {
		t.Fatalf("sessions: %+v", sessions)
	}

	expect(t, c.do("GET", "/api/somas/"+testSoma+"/tree", ""), 200, "")
	srv := decode[map[string]any](t, c.do("GET", "/api/admin/server", ""))
	settings := srv["settings"].(map[string]any)
	if srv["version"] != "test" || settings["DENDRONOTES_ADMIN_PASSWORD_FILE"] != "(set)" {
		t.Errorf("server: %v", srv)
	}
	if open, _ := srv["open"].([]any); len(open) != 1 || open[0].(map[string]any)["name"] != "Test" {
		t.Errorf("open somas: %v", srv["open"])
	}
	if strings.Contains(c.do("GET", "/api/admin/server", "").Body.String(), testPassword) {
		t.Error("server status reveals the admin password")
	}

	expect(t, c.do("DELETE", "/api/admin/sessions/"+strconv.FormatInt(sessions.Current, 10), ""), 200, "")
	expect(t, c.do("GET", "/api/admin/server", ""), 401, "unauthorized")

	c.login()
	audit := decode[struct{ Entries []store.AuditEntry }](t, c.do("GET", "/api/admin/audit?limit=5", "")).Entries
	if len(audit) == 0 || audit[0].Action != "login" {
		t.Errorf("audit: %+v", audit)
	}
}
