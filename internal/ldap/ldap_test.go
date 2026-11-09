package ldap

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"dendronotes/internal/config"
)

func TestGUID(t *testing.T) {
	// {3F2504E0-4F89-11D3-9A0C-0305E82C3301} as Active Directory stores it.
	raw := []byte{0xE0, 0x04, 0x25, 0x3F, 0x89, 0x4F, 0xD3, 0x11, 0x9A, 0x0C, 0x03, 0x05, 0xE8, 0x2C, 0x33, 0x01}
	if got := guid(raw); got != "3f2504e0-4f89-11d3-9a0c-0305e82c3301" {
		t.Errorf("guid: %s", got)
	}
}

func TestInAny(t *testing.T) {
	e := Entry{Groups: []Group{
		{Name: "Notes Admins", DN: "CN=Notes Admins,OU=Groups,DC=example,DC=org"},
		{Name: "Garden", DN: "CN=Garden,OU=dendronotes,DC=example,DC=org"},
	}}
	for _, want := range []string{"notes admins", "cn=notes admins, ou=groups, dc=example, dc=org", "Garden"} {
		if !e.InAny([]string{"Nobody", want}) {
			t.Errorf("%q not found", want)
		}
	}
	for _, other := range []string{"Admins", "CN=Notes Admins,OU=Other,DC=example,DC=org", "CN=Garden"} {
		if e.InAny([]string{other}) {
			t.Errorf("%q matched", other)
		}
	}
	if (Entry{}).InAny([]string{"Garden"}) {
		t.Error("no groups matched")
	}
}

func TestRefusesEmptyPasswordWithoutAsking(t *testing.T) {
	// Nothing listens there: an empty password must fail before connecting.
	c, err := New(config.LDAP{URL: "ldap://127.0.0.1:1", BaseDN: "DC=example,DC=org", UserFilter: "(uid={login})"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Authenticate(context.Background(), "erik", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("empty password: %v", err)
	}
	if _, err := c.Authenticate(context.Background(), "erik", "secret"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("unreachable directory: %v", err)
	}
}

// TestDirectory runs against a real directory when DENDRONOTES_TEST_LDAP_URL
// is set (see the README of the scratch setup): a service account, a person
// with a password and a group below the group base.
func TestDirectory(t *testing.T) {
	env := func(k string) string { return os.Getenv("DENDRONOTES_TEST_LDAP_" + k) }
	if env("URL") == "" {
		t.Skip("DENDRONOTES_TEST_LDAP_URL not set")
	}
	cfg := config.LDAP{
		URL: env("URL"), StartTLS: env("START_TLS") == "true", CAPEM: []byte(env("CA_PEM")),
		BindDN: env("BIND_DN"), BindPassword: config.Secret(env("BIND_PASSWORD")), BaseDN: env("BASE_DN"),
		UserFilter: config.DefaultLDAPUserFilter, UsernameAttr: config.DefaultLDAPUsernameAttr,
		NameAttr: config.DefaultLDAPNameAttr, EmailAttr: config.DefaultLDAPEmailAttr, IDAttr: config.DefaultLDAPIDAttr,
		GroupBaseDN: env("GROUP_BASE_DN"), GroupFilter: config.DefaultLDAPGroupFilter, GroupNameAttr: "cn",
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	e, err := c.Authenticate(ctx, env("USER"), env("PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == "" || !strings.EqualFold(e.Username, env("USER")) || len(e.ID) != 36 {
		t.Errorf("entry: %+v", e)
	}
	synced := map[string]bool{}
	for _, g := range e.Groups {
		synced[g.Name] = g.Synced
	}
	if want := env("EXPECT_GROUP"); want != "" && !synced[want] {
		t.Errorf("group %q missing or not synced: %+v", want, e.Groups)
	}
	if nested := env("EXPECT_NESTED"); nested != "" && !synced[nested] {
		t.Errorf("nested group %q missing: %+v", nested, e.Groups)
	}
	if _, err := c.Authenticate(ctx, env("USER"), env("PASSWORD")+"x"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password: %v", err)
	}
	if _, err := c.Authenticate(ctx, "nobody-"+env("USER"), env("PASSWORD")); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown account: %v", err)
	}
	if _, err := c.Authenticate(ctx, "*", env("PASSWORD")); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("a wildcard must not match: %v", err)
	}
}
