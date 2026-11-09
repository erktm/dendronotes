package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3/driver"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir, discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func user(t *testing.T, s *Store, name string, role Role) User {
	t.Helper()
	u, err := s.CreateUser(context.Background(), User{Username: name, Role: role, Source: SourceLocal, PasswordHash: "h"})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestUsers(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	anna := user(t, s, "Anna", RoleAdmin)
	user(t, s, "ben", RoleUser)
	if _, err := s.CreateUser(ctx, User{Username: "anna", Role: RoleUser, Source: SourceLocal}); !errors.Is(err, ErrTaken) {
		t.Errorf("user names must be unique without regard to case: %v", err)
	}
	if u, err := s.UserByUsername(ctx, "ANNA"); err != nil || u.ID != anna.ID || u.Role != RoleAdmin || u.CreatedAt.IsZero() {
		t.Errorf("lookup: %+v %v", u, err)
	}
	if _, err := s.UserByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}

	name, disabled := "Anna Admin", true
	u, err := s.UpdateUser(ctx, anna.ID, UserUpdate{DisplayName: &name, Disabled: &disabled})
	if err != nil || u.Name() != "Anna Admin" || !u.Disabled {
		t.Errorf("update: %+v %v", u, err)
	}
	if n, _ := s.CountAdmins(ctx); n != 0 {
		t.Errorf("disabled admins must not count: %d", n)
	}
	taken := "BEN"
	if _, err := s.UpdateUser(ctx, anna.ID, UserUpdate{Username: &taken}); !errors.Is(err, ErrTaken) {
		t.Errorf("rename to taken name: %v", err)
	}

	found, _ := s.SearchUsers(ctx, "b", 10)
	if len(found) != 1 || found[0].Username != "ben" {
		t.Errorf("search: %+v", found)
	}
	if found, _ := s.SearchUsers(ctx, "%", 10); len(found) != 0 {
		t.Errorf("LIKE wildcards must be literal: %+v", found)
	}

	oidc, err := s.CreateUser(ctx, User{Username: "carla", Role: RoleUser, Source: SourceOIDC, OIDCIssuer: "https://kc", OIDCSubject: "sub-1"})
	if err != nil {
		t.Fatal(err)
	}
	if u, err := s.UserByOIDC(ctx, "https://kc", "sub-1"); err != nil || u.ID != oidc.ID {
		t.Errorf("OIDC lookup: %+v %v", u, err)
	}
	if _, err := s.CreateUser(ctx, User{Username: "carla2", Role: RoleUser, Source: SourceOIDC, OIDCIssuer: "https://kc", OIDCSubject: "sub-1"}); !errors.Is(err, ErrTaken) {
		t.Errorf("an OIDC identity belongs to one account: %v", err)
	}
}

func TestSessions(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }
	anna := user(t, s, "anna", RoleUser)

	token, err := s.CreateSession(ctx, anna.ID, time.Hour, "10.0.0.1", "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	if u, _, ok := s.LookupSession(ctx, token, time.Hour); !ok || u.ID != anna.ID {
		t.Fatal("fresh session not found")
	}
	if _, _, ok := s.LookupSession(ctx, token+"x", time.Hour); ok {
		t.Error("wrong token accepted")
	}

	// Sliding: activity after 50 minutes extends the session.
	now = now.Add(50 * time.Minute)
	if _, _, ok := s.LookupSession(ctx, token, time.Hour); !ok {
		t.Fatal("session expired too early")
	}
	now = now.Add(50 * time.Minute)
	if _, _, ok := s.LookupSession(ctx, token, time.Hour); !ok {
		t.Error("session did not slide forward")
	}
	now = now.Add(2 * time.Hour)
	if _, _, ok := s.LookupSession(ctx, token, time.Hour); ok {
		t.Error("expired session accepted")
	}

	token, _ = s.CreateSession(ctx, anna.ID, time.Hour, "", "")
	disabled := true
	s.UpdateUser(ctx, anna.ID, UserUpdate{Disabled: &disabled})
	if _, _, ok := s.LookupSession(ctx, token, time.Hour); ok {
		t.Error("session of a disabled account accepted")
	}
	enabled := false
	s.UpdateUser(ctx, anna.ID, UserUpdate{Disabled: &enabled})
	list, _ := s.Sessions(ctx)
	if len(list) != 1 || list[0].Username != "anna" {
		t.Errorf("sessions: %+v", list)
	}
	if err := s.DeleteSessionByID(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := s.LookupSession(ctx, token, time.Hour); ok {
		t.Error("revoked session accepted")
	}

	for i := 0; i < MaxSessionsPerUser+5; i++ {
		if _, err := s.CreateSession(ctx, anna.ID, time.Hour, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if list, _ := s.Sessions(ctx); len(list) != MaxSessionsPerUser {
		t.Errorf("sessions per account not bounded: %d", len(list))
	}
}

func TestSomasAndMembers(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	anna := user(t, s, "anna", RoleUser)
	ben := user(t, s, "ben", RoleUser)

	garden, err := s.CreateSoma(ctx, NewSomaID(), "Garden", "garden", anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSoma(ctx, NewSomaID(), "Other", "garden", ben.ID); !errors.Is(err, ErrTaken) {
		t.Errorf("folders must be unique: %v", err)
	}
	if role, _ := s.MemberRole(ctx, garden.ID, anna.ID); role != MemberOwner {
		t.Errorf("creator should own the soma: %q", role)
	}
	if role, _ := s.MemberRole(ctx, garden.ID, ben.ID); role != "" {
		t.Errorf("non-member has role %q", role)
	}

	if err := s.SetMember(ctx, garden.ID, ben.ID, MemberViewer); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMember(ctx, garden.ID, anna.ID, MemberViewer); err != nil {
		t.Fatal(err)
	}
	if role, _ := s.MemberRole(ctx, garden.ID, anna.ID); role != MemberOwner {
		t.Error("SetMember must not demote the owner")
	}
	if err := s.SetMember(ctx, garden.ID, ben.ID, MemberOwner); err == nil {
		t.Error("owner role only via transfer")
	}
	mine, _ := s.SomasOf(ctx, ben.ID)
	if len(mine) != 1 || mine[0].Role != MemberViewer || mine[0].Owner.Username != "anna" {
		t.Errorf("ben's somas: %+v", mine)
	}

	if err := s.TransferSoma(ctx, garden.ID, ben.ID); err != nil {
		t.Fatal(err)
	}
	members, _ := s.Members(ctx, garden.ID)
	if len(members) != 2 || members[0].Username != "ben" || members[0].Role != MemberOwner || members[1].Role != MemberEditor {
		t.Errorf("after transfer: %+v", members)
	}
	all, _ := s.AllSomas(ctx)
	if len(all) != 1 || all[0].Owner.Username != "ben" || all[0].Members != 2 {
		t.Errorf("all somas: %+v", all)
	}

	if err := s.DeleteUser(ctx, anna.ID); err != nil {
		t.Fatal(err)
	}
	if members, _ := s.Members(ctx, garden.ID); len(members) != 1 {
		t.Errorf("memberships of a deleted account remain: %+v", members)
	}
	if err := s.DeleteSoma(ctx, garden.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SomaByID(ctx, garden.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted soma: %v", err)
	}
}

func TestSomaIDs(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewSomaID()
		if len(id) != 10 || strings.Trim(id, "abcdefghijklmnopqrstuvwxyz234567") != "" || seen[id] {
			t.Fatalf("bad id %q", id)
		}
		seen[id] = true
	}
}

func TestAuditLog(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	for _, a := range []string{"login", "user.create", "login.failed"} {
		s.Audit(ctx, AuditEntry{Actor: "anna", Action: a, IP: "10.0.0.1"})
	}
	all, _ := s.AuditLog(ctx, 0, 10)
	if len(all) != 3 || all[0].Action != "login.failed" {
		t.Fatalf("audit log: %+v", all)
	}
	older, _ := s.AuditLog(ctx, all[1].ID, 10)
	if len(older) != 1 || older[0].Action != "login" {
		t.Errorf("paging: %+v", older)
	}
}

func TestReopenAndVersionChecks(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	user(t, s, "anna", RoleAdmin)
	s.Close()

	s = open(t, dir)
	if u, err := s.UserByUsername(context.Background(), "anna"); err != nil || u.Role != RoleAdmin {
		t.Fatalf("data lost on reopen: %+v %v", u, err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := Open(dir, discard); err == nil || !strings.Contains(err.Error(), "newer release") {
		t.Errorf("a newer schema must be refused, not touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Error(err)
	}
}

func TestAvatars(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	anna := user(t, s, "anna", RoleAdmin)
	ben := user(t, s, "ben", RoleUser)
	cleo := user(t, s, "cleo", RoleUser)

	// Without a design every account gets a color from its id.
	if a := anna.Avatar; a.Pattern != "plain" || a.Color != int(anna.ID%AvatarColors) || a.Photo != 0 || a.Initials != "" {
		t.Errorf("default avatar: %+v", a)
	}

	u, err := s.SetAvatarStyle(ctx, ben.ID, AvatarStyle{Color: 5, Pattern: "rings", Initials: " bö "})
	if err != nil || u.Avatar != (Avatar{Color: 5, Pattern: "rings", Initials: "BÖ"}) {
		t.Fatalf("set style: %+v %v", u.Avatar, err)
	}
	for _, bad := range []AvatarStyle{
		{Color: AvatarColors, Pattern: "plain"},
		{Color: -1, Pattern: "plain"},
		{Color: 0, Pattern: "<script>"},
		{Color: 0, Pattern: "plain", Initials: "ABC"},
		{Color: 0, Pattern: "plain", Initials: "A!"},
	} {
		if _, err := s.SetAvatarStyle(ctx, ben.ID, bad); !errors.Is(err, ErrInvalidAvatar) {
			t.Errorf("%+v: want ErrInvalidAvatar, got %v", bad, err)
		}
	}

	// Photos get a new version on every upload and leave the design alone.
	u, err = s.SetAvatarPhoto(ctx, ben.ID, []byte("one"))
	if err != nil || u.Avatar.Photo == 0 || u.Avatar.Pattern != "rings" {
		t.Fatalf("set photo: %+v %v", u.Avatar, err)
	}
	first := u.Avatar.Photo
	u, _ = s.SetAvatarPhoto(ctx, ben.ID, []byte("two"))
	if u.Avatar.Photo <= first {
		t.Errorf("version did not move on: %d → %d", first, u.Avatar.Photo)
	}
	if img, v, err := s.AvatarPhoto(ctx, ben.ID); err != nil || string(img) != "two" || v != u.Avatar.Photo {
		t.Errorf("photo: %q %d %v", img, v, err)
	}
	// The session lookup carries the avatar as well.
	token, err := s.CreateSession(ctx, ben.ID, time.Hour, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if su, _, ok := s.LookupSession(ctx, token, time.Hour); !ok || su.Avatar != u.Avatar {
		t.Errorf("session user avatar: %+v %v", su.Avatar, ok)
	}
	u, err = s.RemoveAvatarPhoto(ctx, ben.ID)
	if err != nil || u.Avatar.Photo != 0 {
		t.Fatalf("remove photo: %+v %v", u.Avatar, err)
	}
	if _, _, err := s.AvatarPhoto(ctx, ben.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("removed photo: %v", err)
	}
	if _, err := s.SetAvatarPhoto(ctx, 9999, []byte("x")); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown account: %v", err)
	}

	// Members carry their avatars; sharing a soma is what links people.
	if _, err := s.CreateSoma(ctx, "somaaaaaaa", "Shared", "shared", anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMember(ctx, "somaaaaaaa", ben.ID, MemberViewer); err != nil {
		t.Fatal(err)
	}
	members, err := s.Members(ctx, "somaaaaaaa")
	if err != nil || len(members) != 2 || members[1].Avatar == nil || members[1].Avatar.Pattern != "rings" {
		t.Fatalf("members: %+v %v", members, err)
	}
	for _, c := range []struct {
		a, b int64
		want bool
	}{{anna.ID, ben.ID, true}, {ben.ID, anna.ID, true}, {anna.ID, cleo.ID, false}, {cleo.ID, cleo.ID, false}} {
		if got, err := s.SharesSoma(ctx, c.a, c.b); err != nil || got != c.want {
			t.Errorf("SharesSoma(%d, %d) = %v %v", c.a, c.b, got, err)
		}
	}
}

func TestMigrationKeepsAccounts(t *testing.T) {
	// A state.db at schema version 1 (before avatars) opens and keeps its accounts.
	dir := t.TempDir()
	db, err := driver.Open("file:" + filepath.ToSlash(filepath.Join(dir, FileName)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0] + `; PRAGMA user_version = 1;
		INSERT INTO users (username, role, source, created_at) VALUES ('anna', 'admin', 'local', 1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s := open(t, dir)
	u, err := s.UserByUsername(context.Background(), "anna")
	if err != nil || u.Avatar.Pattern != "plain" {
		t.Fatalf("after migration: %+v %v", u, err)
	}
	if _, err := s.SetAvatarPhoto(context.Background(), u.ID, []byte("x")); err != nil {
		t.Error(err)
	}
}

func TestRolesAndGroups(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	anna := user(t, s, "anna", RoleAdmin)
	ben := user(t, s, "ben", RoleUser)
	cleo := user(t, s, "cleo", RoleUser)
	dora := user(t, s, "dora", RoleUser)
	off := true
	if _, err := s.UpdateUser(ctx, dora.ID, UserUpdate{Disabled: &off}); err != nil {
		t.Fatal(err)
	}
	const soma = "somaaaaaaa"
	if _, err := s.CreateSoma(ctx, soma, "Garden", "garden", anna.ID); err != nil {
		t.Fatal(err)
	}
	role := func(u User) string {
		t.Helper()
		r, err := s.EffectiveRole(ctx, soma, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	// Roles are ordered; owner cannot be given away by sharing.
	for _, r := range []string{MemberViewer, MemberWriter, MemberContributor, MemberEditor, MemberOwner} {
		if roleOfRank(RoleRank(r)) != r {
			t.Errorf("rank round trip of %s", r)
		}
	}
	if IsShareRole(MemberOwner) || !IsShareRole(MemberWriter) || IsShareRole("admin") {
		t.Error("share roles")
	}
	if err := s.SetMember(ctx, soma, ben.ID, MemberOwner); err == nil {
		t.Error("owner given by sharing")
	}

	// Personal shares with the new roles.
	if role(anna) != MemberOwner || role(ben) != "" {
		t.Fatalf("before sharing: %q %q", role(anna), role(ben))
	}
	if err := s.SetMember(ctx, soma, ben.ID, MemberWriter); err != nil {
		t.Fatal(err)
	}
	if role(ben) != MemberWriter {
		t.Errorf("ben: %q", role(ben))
	}

	// Groups: the highest role counts.
	team, err := s.CreateGroup(ctx, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGroup(ctx, "team"); !errors.Is(err, ErrTaken) {
		t.Errorf("duplicate group name: %v", err)
	}
	for _, u := range []User{ben, cleo} {
		if err := s.AddGroupMember(ctx, team.ID, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddGroupMember(ctx, team.ID, 9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown account: %v", err)
	}
	if err := s.SetSomaGroup(ctx, soma, team.ID, MemberContributor); err != nil {
		t.Fatal(err)
	}
	if role(ben) != MemberContributor || role(cleo) != MemberContributor {
		t.Errorf("through the group: ben %q, cleo %q", role(ben), role(cleo))
	}
	somas, err := s.SomasOf(ctx, cleo.ID)
	if err != nil || len(somas) != 1 || somas[0].Role != MemberContributor || somas[0].Direct {
		t.Fatalf("cleo's somas: %+v %v", somas, err)
	}
	if somas, _ := s.SomasOf(ctx, ben.ID); len(somas) != 1 || !somas[0].Direct {
		t.Errorf("ben's somas: %+v", somas)
	}
	if ok, _ := s.SharesSoma(ctx, anna.ID, cleo.ID); !ok {
		t.Error("anna and cleo share the soma through the group")
	}

	// Everyone: built in, all enabled accounts, cannot be changed.
	groups, err := s.Groups(ctx)
	if err != nil || len(groups) != 2 || !groups[0].Everyone || groups[0].Members != 3 || groups[1].Members != 2 {
		t.Fatalf("groups: %+v %v", groups, err)
	}
	everyone := groups[0].ID
	if _, err := s.RenameGroup(ctx, everyone, "All"); !errors.Is(err, ErrBuiltinGroup) {
		t.Errorf("rename Everyone: %v", err)
	}
	if err := s.DeleteGroup(ctx, everyone); !errors.Is(err, ErrBuiltinGroup) {
		t.Errorf("delete Everyone: %v", err)
	}
	if err := s.AddGroupMember(ctx, everyone, ben.ID); !errors.Is(err, ErrBuiltinGroup) {
		t.Errorf("members of Everyone: %v", err)
	}
	if _, err := s.CreateGroup(ctx, "EVERYONE"); !errors.Is(err, ErrTaken) {
		t.Errorf("second Everyone: %v", err)
	}
	if err := s.SetSomaGroup(ctx, soma, everyone, MemberViewer); err != nil {
		t.Fatal(err)
	}
	other := user(t, s, "erin", RoleUser) // also later accounts
	if role(other) != MemberViewer || role(dora) != "" || role(ben) != MemberContributor {
		t.Errorf("with everyone: erin %q, dora (disabled) %q, ben %q", role(other), role(dora), role(ben))
	}
	grants, err := s.SomaGroups(ctx, soma)
	if err != nil || len(grants) != 2 || !grants[0].Everyone || grants[1].Role != MemberContributor {
		t.Fatalf("grants: %+v %v", grants, err)
	}
	if ids, _ := s.GroupSomas(ctx, team.ID); len(ids) != 1 || ids[0] != soma {
		t.Errorf("group somas: %v", ids)
	}
	if found, _ := s.SearchGroups(ctx, "ev", 10); len(found) != 1 || !found[0].Everyone {
		t.Errorf("search: %+v", found)
	}
	if infos, _ := s.AllSomas(ctx); len(infos) != 1 || infos[0].Groups != 2 || infos[0].Members != 2 {
		t.Errorf("admin somas: %+v", infos)
	}

	// Taking the group away leaves the personal share.
	if err := s.RemoveSomaGroup(ctx, soma, everyone); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveGroupMember(ctx, team.ID, cleo.ID); err != nil {
		t.Fatal(err)
	}
	if role(cleo) != "" || role(other) != "" {
		t.Errorf("after removing: cleo %q, erin %q", role(cleo), role(other))
	}
	if err := s.DeleteGroup(ctx, team.ID); err != nil {
		t.Fatal(err)
	}
	if role(ben) != MemberWriter {
		t.Errorf("ben after the group is gone: %q", role(ben))
	}
	if grants, _ := s.SomaGroups(ctx, soma); len(grants) != 0 {
		t.Errorf("grants of a deleted group: %+v", grants)
	}
}

func TestMigrationKeepsShares(t *testing.T) {
	// A state.db at schema version 2 keeps its somas and shares.
	dir := t.TempDir()
	db, err := driver.Open("file:" + filepath.ToSlash(filepath.Join(dir, FileName)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0] + `;` + migrations[1] + `; PRAGMA user_version = 2;
		INSERT INTO users (id, username, role, source, created_at) VALUES (1, 'anna', 'admin', 'local', 1), (2, 'ben', 'user', 'local', 1), (3, 'cleo', 'user', 'local', 1);
		INSERT INTO somas (id, name, folder, owner_id, created_at) VALUES ('somaaaaaaa', 'Garden', 'garden', 1, 1);
		INSERT INTO soma_members (soma_id, user_id, role) VALUES ('somaaaaaaa', 1, 'owner'), ('somaaaaaaa', 2, 'editor'), ('somaaaaaaa', 3, 'viewer')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s := open(t, dir)
	ctx := context.Background()
	for id, want := range map[int64]string{1: MemberOwner, 2: MemberEditor, 3: MemberViewer} {
		if got, err := s.EffectiveRole(ctx, "somaaaaaaa", id); err != nil || got != want {
			t.Errorf("user %d: %q %v, want %q", id, got, err, want)
		}
	}
	if err := s.SetMember(ctx, "somaaaaaaa", 3, MemberWriter); err != nil {
		t.Errorf("new roles after migrating: %v", err)
	}
	if groups, err := s.Groups(ctx); err != nil || len(groups) != 1 || !groups[0].Everyone || groups[0].Name != "Everyone" {
		t.Errorf("Everyone after migrating: %+v %v", groups, err)
	}
}

func TestDirectoryAccountsAndGroups(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	local := user(t, s, "team", RoleUser) // a local group name clash comes later

	anna, err := s.CreateUser(ctx, User{Username: "anna", Role: RoleUser, Source: SourceLDAP, LDAPID: "guid-anna"})
	if err != nil {
		t.Fatal(err)
	}
	if u, err := s.UserByLDAP(ctx, "guid-anna"); err != nil || u.ID != anna.ID || u.Source != SourceLDAP {
		t.Fatalf("by directory id: %+v %v", u, err)
	}
	if _, err := s.CreateUser(ctx, User{Username: "anna2", Role: RoleUser, Source: SourceLDAP, LDAPID: "guid-anna"}); !errors.Is(err, ErrTaken) {
		t.Errorf("same directory id twice: %v", err)
	}
	if _, err := s.UserByLDAP(ctx, "guid-none"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}

	// A local group with the name of a directory group stays; the directory one gets " (AD)".
	if _, err := s.CreateGroup(ctx, "Team"); err != nil {
		t.Fatal(err)
	}
	changed, err := s.SyncDirectoryGroups(ctx, anna.ID, []DirectoryGroup{{ID: "g-team", Name: "Team"}, {ID: "g-garden", Name: "Garden"}})
	if err != nil || len(changed) != 2 {
		t.Fatalf("first sync: %v %v", changed, err)
	}
	byName := func() map[string]Group {
		list, _ := s.Groups(ctx)
		m := map[string]Group{}
		for _, g := range list {
			m[g.Name] = g
		}
		return m
	}
	groups := byName()
	if g := groups["Team (AD)"]; g.Source != "ldap" || g.Members != 1 || groups["Team"].Source != "local" || groups["Garden"].Members != 1 {
		t.Fatalf("groups after sync: %+v", groups)
	}
	// Nothing changed: nothing reported.
	if changed, _ := s.SyncDirectoryGroups(ctx, anna.ID, []DirectoryGroup{{ID: "g-team", Name: "Team"}, {ID: "g-garden", Name: "Garden"}}); len(changed) != 0 {
		t.Errorf("unchanged sync reported %v", changed)
	}
	// Renamed in the directory, left another one.
	changed, err = s.SyncDirectoryGroups(ctx, anna.ID, []DirectoryGroup{{ID: "g-garden", Name: "Allotment"}})
	if err != nil || len(changed) != 1 {
		t.Fatalf("second sync: %v %v", changed, err)
	}
	groups = byName()
	if g, ok := groups["Allotment"]; !ok || g.Members != 1 || groups["Team (AD)"].Members != 0 || groups["Garden"].ID != 0 {
		t.Fatalf("after renaming and leaving: %+v", groups)
	}
	// Directory groups are read-only in dendronotes, except for deleting.
	ad := groups["Allotment"]
	if _, err := s.RenameGroup(ctx, ad.ID, "Mine"); !errors.Is(err, ErrDirectoryGroup) {
		t.Errorf("rename: %v", err)
	}
	if err := s.AddGroupMember(ctx, ad.ID, local.ID); !errors.Is(err, ErrDirectoryGroup) {
		t.Errorf("add member: %v", err)
	}
	if err := s.RemoveGroupMember(ctx, ad.ID, anna.ID); !errors.Is(err, ErrDirectoryGroup) {
		t.Errorf("remove member: %v", err)
	}
	if m, err := s.GroupMembers(ctx, ad.ID); err != nil || len(m) != 1 {
		t.Errorf("members of a directory group: %+v %v", m, err)
	}
	if err := s.DeleteGroup(ctx, ad.ID); err != nil {
		t.Errorf("delete a directory group: %v", err)
	}
}

func TestMigrationRebuildsUsers(t *testing.T) {
	// A state.db at version 3 with everything that refers to accounts keeps it.
	dir := t.TempDir()
	db, err := driver.Open("file:" + filepath.ToSlash(filepath.Join(dir, FileName)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0] + `;` + migrations[1] + `;` + migrations[2] + `; PRAGMA user_version = 3;
		INSERT INTO users (id, username, role, source, created_at, avatar_style, avatar_photo)
			VALUES (1, 'anna', 'admin', 'local', 1, '{"color":3,"pattern":"rings","initials":""}', 7), (2, 'ben', 'user', 'oidc', 1, '', 0);
		UPDATE users SET oidc_issuer = 'https://sso', oidc_subject = 'ben' WHERE id = 2;
		INSERT INTO sessions (token_hash, user_id, created_at, expires_at, seen_at) VALUES ('h', 2, 1, 9999999999, 1);
		INSERT INTO somas (id, name, folder, owner_id, created_at) VALUES ('somaaaaaaa', 'Garden', 'garden', 1, 1);
		INSERT INTO soma_members (soma_id, user_id, role) VALUES ('somaaaaaaa', 1, 'owner'), ('somaaaaaaa', 2, 'writer');
		INSERT INTO groups (name, created_at) VALUES ('Team', 1);
		INSERT INTO group_members (group_id, user_id) VALUES (2, 2);
		INSERT INTO avatars (user_id, image) VALUES (1, x'89');`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s := open(t, dir)
	ctx := context.Background()
	anna, err := s.UserByUsername(ctx, "anna")
	if err != nil || anna.Avatar.Pattern != "rings" || anna.Avatar.Photo != 7 || anna.Role != RoleAdmin {
		t.Fatalf("anna after the rebuild: %+v %v", anna, err)
	}
	if ben, err := s.UserByOIDC(ctx, "https://sso", "ben"); err != nil || ben.ID != 2 {
		t.Errorf("ben by provider: %+v %v", ben, err)
	}
	if r, err := s.EffectiveRole(ctx, "somaaaaaaa", 2); err != nil || r != MemberWriter {
		t.Errorf("ben's role: %q %v", r, err)
	}
	if img, _, err := s.AvatarPhoto(ctx, 1); err != nil || len(img) != 1 {
		t.Errorf("avatar: %v", err)
	}
	// Foreign keys still hold: deleting an account takes its references along.
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = 2; DELETE FROM users WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM soma_members WHERE user_id = 2) + (SELECT COUNT(*) FROM group_members WHERE user_id = 2)`).Scan(&left); err != nil || left != 0 {
		t.Errorf("cascade after the rebuild: %d %v", left, err)
	}
	var fk int
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign keys are off after migrating: %d %v", fk, err)
	}
}
