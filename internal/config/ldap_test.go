package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLDAP(t *testing.T) {
	soma, data := dirs(t)
	pw := filepath.Join(t.TempDir(), "bind")
	if err := os.WriteFile(pw, []byte("service secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := func() map[string]string {
		return map[string]string{
			EnvSomasDir: soma, EnvDataDir: data,
			EnvLDAPURL: "ldaps://dc1.example.org", EnvLDAPBaseDN: "DC=example,DC=org",
			EnvLDAPBindDN: "CN=svc-dendronotes,OU=Service,DC=example,DC=org", EnvLDAPBindPassword: pw,
		}
	}

	c, err := Load(env(base()))
	l := c.Auth.LDAP
	if err != nil || !l.Enabled() || l.BindPassword.Reveal() != "service secret" || l.UserFilter != DefaultLDAPUserFilter ||
		l.GroupFilter != DefaultLDAPGroupFilter || l.UsernameAttr != "sAMAccountName" || l.IDAttr != "objectGUID" ||
		l.NeedsGroups() {
		t.Fatalf("defaults: %+v %v", l, err)
	}
	if c.Describe(EnvLDAPBindPassword) != "(set)" || c.Describe(EnvLDAPURL) != "ldaps://dc1.example.org" {
		t.Errorf("describe: %q %q", c.Describe(EnvLDAPBindPassword), c.Describe(EnvLDAPURL))
	}

	m := base()
	m[EnvLDAPGroupBaseDN] = "OU=dendronotes,OU=Groups,DC=example,DC=org"
	m[EnvLDAPAdminGroups] = "CN=dn-admins,OU=Groups,DC=example,DC=org; Notes admins"
	m[EnvLocalLogin] = "false"
	c, err = Load(env(m))
	if err != nil || !c.Auth.LDAP.NeedsGroups() || len(c.Auth.LDAP.AdminGroups) != 2 || c.Auth.LocalLogin {
		t.Fatalf("groups: %+v %v", c.Auth.LDAP, err)
	}

	// Plain ldap:// only for this machine; StartTLS for others.
	m = base()
	m[EnvLDAPURL] = "ldap://127.0.0.1:3389"
	if _, err := Load(env(m)); err != nil {
		t.Errorf("plain ldap on localhost: %v", err)
	}
	m[EnvLDAPURL] = "ldap://dc1.example.org"
	if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), "unencrypted") {
		t.Errorf("plain ldap to another host: %v", err)
	}
	m[EnvLDAPStartTLS] = "true"
	if _, err := Load(env(m)); err != nil {
		t.Errorf("ldap with StartTLS: %v", err)
	}

	for name, change := range map[string]map[string]string{
		"no base":          {EnvLDAPBaseDN: ""},
		"bind dn alone":    {EnvLDAPBindPassword: ""},
		"wrong scheme":     {EnvLDAPURL: "https://dc1.example.org"},
		"with a path":      {EnvLDAPURL: "ldaps://dc1.example.org/DC=example"},
		"starttls twice":   {EnvLDAPStartTLS: "true"},
		"bad starttls":     {EnvLDAPURL: "ldap://localhost", EnvLDAPStartTLS: "maybe"},
		"filter w/o login": {EnvLDAPUserFilter: "(uid=erik)"},
		"group w/o dn":     {EnvLDAPGroupFilter: "(objectClass=group)"},
		"no parentheses":   {EnvLDAPUserFilter: "uid={login}"},
		"missing ca file":  {EnvLDAPCAFile: filepath.Join(t.TempDir(), "none.pem")},
		"proxy mode":       {EnvAuthMode: "proxy", EnvTrustedProxies: "10.0.0.0/8"},
	} {
		m := base()
		for k, v := range change {
			m[k] = v
		}
		if _, err := Load(env(m)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
