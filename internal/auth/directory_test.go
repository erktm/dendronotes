package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dendronotes/internal/clientip"
	"dendronotes/internal/ldap"
	"dendronotes/internal/store"
)

// fakeDirectory knows accounts by every login (user name, UPN …).
type fakeDirectory struct {
	accounts map[string]fakeAccount
	down     bool
}

type fakeAccount struct {
	password string
	entry    ldap.Entry
}

func (d *fakeDirectory) Authenticate(_ context.Context, login, password string) (ldap.Entry, error) {
	if d.down {
		return ldap.Entry{}, fmt.Errorf("%w: connection refused", ldap.ErrUnavailable)
	}
	a, ok := d.accounts[strings.ToLower(login)]
	if !ok || password == "" || password != a.password {
		return ldap.Entry{}, ldap.ErrInvalidCredentials
	}
	return a.entry, nil
}

func (d *fakeDirectory) Lookup(_ context.Context, id string) (ldap.Entry, error) {
	if d.down {
		return ldap.Entry{}, fmt.Errorf("%w: connection refused", ldap.ErrUnavailable)
	}
	for _, a := range d.accounts {
		if a.entry.ID == id {
			return a.entry, nil
		}
	}
	return ldap.Entry{}, ldap.ErrNotFound
}

func newDirectoryFixture(t *testing.T, dir *fakeDirectory, change func(*Config)) *fixture {
	t.Helper()
	st, err := store.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := Config{
		Mode: ModeLocal, SessionTTL: time.Hour, CookieSecure: "auto", HashParams: cheap, AdminUsername: "admin",
		Directory: dir, DirectoryGroups: true,
	}
	if change != nil {
		change(&cfg)
	}
	a, err := New(cfg, st, clientip.New(nil), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{a: a, st: st}
}

var nextIP atomic.Int32

// signIn posts a login from a new client address (no IP rate limit in the way).
func signIn(a *Authenticator, login, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": login, "password": password})
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	n := nextIP.Add(1)
	req.RemoteAddr = fmt.Sprintf("10.9.%d.%d:1", n/250, n%250+1)
	rec := httptest.NewRecorder()
	a.HandleLogin(rec, req)
	return rec
}

func signedInAs(t *testing.T, rec *httptest.ResponseRecorder) store.User {
	t.Helper()
	if rec.Code != 200 {
		t.Fatalf("sign-in: %d %s", rec.Code, rec.Body)
	}
	var me MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	return me.User
}

func errorCode(rec *httptest.ResponseRecorder) string {
	var e struct{ Error string }
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	return fmt.Sprintf("%d %s", rec.Code, e.Error)
}

func TestDirectorySignIn(t *testing.T) {
	ben := ldap.Entry{ID: "guid-ben", DN: "CN=Ben,OU=People,DC=corp", Username: "ben", Name: "Ben Miller", Email: "ben@corp.example",
		Groups: []ldap.Group{
			{ID: "g1", Name: "Garden", DN: "CN=Garden,OU=dendronotes,DC=corp", Synced: true},
			{ID: "g2", Name: "Domain Users", DN: "CN=Domain Users,CN=Users,DC=corp"},
		}}
	erik := ldap.Entry{ID: "guid-erik", DN: "CN=Erik,OU=People,DC=corp", Username: "erik", Name: "Erik"}
	dir := &fakeDirectory{accounts: map[string]fakeAccount{
		"ben":               {"ben's ad password", ben},
		"ben@corp.example":  {"ben's ad password", ben},
		"erik":              {"erik's ad password", erik},
		"erik@corp.example": {"erik's ad password", erik},
	}}
	var provisioned, synced []string
	f := newDirectoryFixture(t, dir, func(c *Config) {
		c.OnProvision = func(_ context.Context, u store.User) error { provisioned = append(provisioned, u.Username); return nil }
		c.OnDirectoryGroups = func(_ context.Context, ids []int64) { synced = append(synced, fmt.Sprint(len(ids))) }
	})
	ctx := context.Background()
	// A local account "erik" with its own password.
	f.addUser(t, "erik", "erik's local password", store.RoleUser)

	// First sign-in creates the account, with name, e-mail and synced groups.
	u := signedInAs(t, signIn(f.a, "ben", "ben's ad password"))
	if u.Source != store.SourceLDAP || u.Username != "ben" || u.DisplayName != "Ben Miller" || u.Email != "ben@corp.example" || u.Role != store.RoleUser {
		t.Fatalf("account: %+v", u)
	}
	if len(provisioned) != 1 || len(synced) != 1 {
		t.Errorf("provisioned %v, synced %v", provisioned, synced)
	}
	groups, _ := f.st.Groups(ctx)
	if len(groups) != 2 || groups[1].Name != "Garden" || groups[1].Source != "ldap" || groups[1].Members != 1 {
		t.Fatalf("groups (only those below the base): %+v", groups)
	}
	// The same person again – also by e-mail – is the same account.
	if again := signedInAs(t, signIn(f.a, "BEN@corp.example", "ben's ad password")); again.ID != u.ID {
		t.Errorf("second sign-in: %+v", again)
	}
	if len(provisioned) != 1 {
		t.Errorf("provisioned twice: %v", provisioned)
	}

	// Local accounts come first: "erik" is the local one, the directory Erik
	// signs in by e-mail and becomes erik-2.
	if code := errorCode(signIn(f.a, "erik", "erik's ad password")); code != "401 invalid_credentials" {
		t.Errorf("directory password for a local account: %s", code)
	}
	if local := signedInAs(t, signIn(f.a, "erik", "erik's local password")); local.Source != store.SourceLocal {
		t.Errorf("local erik: %+v", local)
	}
	ad := signedInAs(t, signIn(f.a, "erik@corp.example", "erik's ad password"))
	if ad.Username != "erik-2" || ad.Source != store.SourceLDAP {
		t.Errorf("directory erik: %+v", ad)
	}

	// Wrong passwords, unknown people, empty passwords.
	for _, c := range [][2]string{{"ben", "wrong"}, {"nobody", "x"}, {"ben", ""}} {
		if code := errorCode(signIn(f.a, c[0], c[1])); code != "401 invalid_credentials" {
			t.Errorf("%v: %s", c, code)
		}
	}
	// A directory that cannot be reached says so.
	dir.down = true
	if code := errorCode(signIn(f.a, "ben", "ben's ad password")); code != "503 directory_unavailable" {
		t.Errorf("directory down: %s", code)
	}
	dir.down = false

	// Leaving the group in the directory ends the membership at the next sign-in.
	acc := dir.accounts["ben"]
	acc.entry.Groups = acc.entry.Groups[1:]
	dir.accounts["ben"] = acc
	signedInAs(t, signIn(f.a, "ben", "ben's ad password"))
	if groups, _ := f.st.Groups(ctx); groups[1].Members != 0 {
		t.Errorf("still a member: %+v", groups)
	}

	// Disabled in dendronotes: refused, even with the right password.
	off := true
	if _, err := f.st.UpdateUser(ctx, ad.ID, store.UserUpdate{Disabled: &off}); err != nil {
		t.Fatal(err)
	}
	if code := errorCode(signIn(f.a, "erik@corp.example", "erik's ad password")); code != "403 account_disabled" {
		t.Errorf("disabled: %s", code)
	}
}

func TestDirectoryGroupsDecide(t *testing.T) {
	admin := ldap.Entry{ID: "guid-ada", DN: "CN=Ada,DC=corp", Username: "ada",
		Groups: []ldap.Group{{ID: "ga", Name: "Notes Admins", DN: "CN=Notes Admins,DC=corp"}}}
	other := ldap.Entry{ID: "guid-otto", DN: "CN=Otto,DC=corp", Username: "otto"}
	dir := &fakeDirectory{accounts: map[string]fakeAccount{"ada": {"pw ada", admin}, "otto": {"pw otto", other}}}
	f := newDirectoryFixture(t, dir, func(c *Config) {
		c.DirectoryAdminGroups = []string{"CN=Notes Admins,DC=corp"}
		c.DirectoryAllowedGroups = []string{"notes admins"}
		c.PasswordsAdminOnly = true // directory sign-in still works
	})
	if u := signedInAs(t, signIn(f.a, "ada", "pw ada")); u.Role != store.RoleAdmin {
		t.Errorf("admin from the group: %+v", u)
	}
	if code := errorCode(signIn(f.a, "otto", "pw otto")); code != "403 account_not_allowed" {
		t.Errorf("not in an allowed group: %s", code)
	}
	// Out of the admin group: back to a normal account.
	acc := dir.accounts["ada"]
	acc.entry.Groups = append(acc.entry.Groups, ldap.Group{Name: "Notes Admins"})
	acc.entry.Groups = acc.entry.Groups[1:]
	dir.accounts["ada"] = acc
	if u := signedInAs(t, signIn(f.a, "ada", "pw ada")); u.Role != store.RoleUser {
		t.Errorf("admin role kept: %+v", u)
	}
	// The password form stays even though local passwords are off.
	rec := httptest.NewRecorder()
	f.a.HandleMethods(rec, httptest.NewRequest("GET", "/api/auth/methods", nil))
	if !strings.Contains(rec.Body.String(), `"local":true`) {
		t.Errorf("methods: %s", rec.Body)
	}
}

func TestDirectorySync(t *testing.T) {
	garden := ldap.Group{ID: "g1", Name: "Garden", DN: "CN=Garden,OU=dendronotes,DC=corp", Synced: true}
	staff := ldap.Group{ID: "g2", Name: "Staff", DN: "CN=Staff,DC=corp"}
	entry := func(id, name string, groups ...ldap.Group) ldap.Entry {
		return ldap.Entry{ID: id, DN: "CN=" + name + ",DC=corp", Username: strings.ToLower(name), Name: name, Groups: groups}
	}
	dir := &fakeDirectory{accounts: map[string]fakeAccount{
		"ben":  {"pw ben", entry("id-ben", "Ben", garden, staff)},
		"ada":  {"pw ada", entry("id-ada", "Ada", staff)},
		"cara": {"pw cara", entry("id-cara", "Cara", staff)},
		"dan":  {"pw dan", entry("id-dan", "Dan", staff)},
	}}
	var signedOut []int64
	var changedGroups int
	f := newDirectoryFixture(t, dir, func(c *Config) {
		c.DirectoryAllowedGroups = []string{"Staff"}
		c.OnSignedOut = func(_ context.Context, id int64) { signedOut = append(signedOut, id) }
		c.OnDirectoryGroups = func(_ context.Context, ids []int64) { changedGroups += len(ids) }
	})
	ctx := context.Background()
	users := map[string]store.User{}
	for _, name := range []string{"ben", "ada", "cara", "dan"} {
		users[name] = signedInAs(t, signIn(f.a, name, "pw "+name))
	}
	if _, _, err := f.st.CreateToken(ctx, users["ada"].ID, "script", store.ScopeRead, time.Time{}); err != nil {
		t.Fatal(err)
	}
	changedGroups = 0

	// Nothing changed: nothing to do.
	res, err := f.a.SyncDirectory(ctx)
	if err != nil || res != (DirectorySync{Accounts: 4}) {
		t.Fatalf("quiet sync: %+v %v", res, err)
	}

	// Ben is renamed and leaves Garden; Ada is gone; Cara is disabled; Dan leaves Staff.
	acc := dir.accounts["ben"]
	acc.entry.Name, acc.entry.Groups = "Ben Miller", []ldap.Group{staff}
	dir.accounts["ben"] = acc
	delete(dir.accounts, "ada")
	acc = dir.accounts["cara"]
	acc.entry.Disabled = true
	dir.accounts["cara"] = acc
	acc = dir.accounts["dan"]
	acc.entry.Groups = nil
	dir.accounts["dan"] = acc

	// Down: nothing happens.
	dir.down = true
	if _, err := f.a.SyncDirectory(ctx); !errors.Is(err, ldap.ErrUnavailable) {
		t.Errorf("directory down: %v", err)
	}
	if n := f.sessions(t, users["ada"].ID); n != 1 {
		t.Errorf("signed out while the directory was down: %d sessions left", n)
	}
	dir.down = false

	res, err = f.a.SyncDirectory(ctx)
	if err != nil || res != (DirectorySync{Accounts: 4, Updated: 1, SignedOut: 3}) {
		t.Fatalf("sync: %+v %v", res, err)
	}
	if u, _ := f.st.UserByID(ctx, users["ben"].ID); u.DisplayName != "Ben Miller" {
		t.Errorf("ben: %+v", u)
	}
	if changedGroups != 1 {
		t.Errorf("changed groups: %d", changedGroups)
	}
	for _, name := range []string{"ada", "cara", "dan"} {
		if n := f.sessions(t, users[name].ID); n != 0 {
			t.Errorf("%s still has %d sessions", name, n)
		}
	}
	if n := f.sessions(t, users["ben"].ID); n != 1 {
		t.Errorf("ben was signed out")
	}
	if tokens, _ := f.st.Tokens(ctx, users["ada"].ID); len(tokens) != 0 {
		t.Errorf("ada keeps her tokens: %+v", tokens)
	}
	if len(signedOut) != 3 {
		t.Errorf("live streams ended for %v", signedOut)
	}
	// Signed out again: nothing left to end.
	if res, _ := f.a.SyncDirectory(ctx); res.SignedOut != 0 {
		t.Errorf("second sync: %+v", res)
	}

	// Most accounts missing at once: a wrong setting, not departures.
	signIn(f.a, "ben", "pw ben")
	dir.accounts = map[string]fakeAccount{"nobody": {"x", entry("id-x", "X", staff)}}
	if _, err := f.a.SyncDirectory(ctx); !errors.Is(err, ErrSuspiciousSync) {
		t.Errorf("suspicious sync: %v", err)
	}
	if n := f.sessions(t, users["ben"].ID); n == 0 {
		t.Error("ben was signed out by a suspicious sync")
	}
}

// sessions counts the sessions of an account.
func (f *fixture) sessions(t *testing.T, userID int64) int {
	t.Helper()
	list, err := f.st.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, s := range list {
		if s.UserID == userID {
			n++
		}
	}
	return n
}
