package config

import (
	"strings"
	"testing"
)

func TestSignup(t *testing.T) {
	soma, data := dirs(t)
	mail := map[string]string{
		EnvSomasDir: soma, EnvDataDir: data, EnvSMTPHost: "smtp.example.org",
		EnvMailFrom: "dendronotes <no-reply@example.org>", EnvPublicURL: "https://notes.example.org",
	}
	with := func(extra map[string]string) (Config, error) {
		m := map[string]string{}
		for k, v := range mail {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return Load(env(m))
	}

	c, err := with(nil)
	if err != nil || c.Signup.Enabled {
		t.Fatalf("off by default: %+v %v", c.Signup, err)
	}
	c, err = with(map[string]string{EnvSignup: "true", EnvSignupDomains: "Example.org, @team.example; example.org"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Signup.Enabled || strings.Join(c.Signup.Domains, ",") != "example.org,team.example" {
		t.Errorf("parsed: %+v", c.Signup)
	}
	if !c.Signup.Allows("anna@Example.ORG") || c.Signup.Allows("anna@other.example") || c.Signup.Allows("no address") {
		t.Errorf("domains: %+v", c.Signup)
	}
	if !(Signup{Enabled: true}).Allows("anyone@anywhere.example") {
		t.Error("without domains every address may sign up")
	}
	if got := c.Describe(EnvSignupDomains); got != "example.org, team.example" {
		t.Errorf("described as %q", got)
	}

	for name, tc := range map[string]struct {
		set  map[string]string
		want string
	}{
		"value":       {map[string]string{EnvSignup: "yes"}, EnvSignup},
		"domain":      {map[string]string{EnvSignup: "true", EnvSignupDomains: "not a domain"}, EnvSignupDomains},
		"domains off": {map[string]string{EnvSignupDomains: "example.org"}, EnvSignupDomains},
		"no mail":     {map[string]string{EnvSignup: "true", EnvSMTPHost: ""}, EnvSMTPHost},
		"admin only":  {map[string]string{EnvSignup: "true", EnvLocalLogin: "false", EnvOIDCIssuer: "https://sso.example.org/realms/main", EnvOIDCClientID: "notes"}, EnvLocalLogin},
	} {
		if _, err := with(tc.set); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error about %s", name, err, tc.want)
		}
	}
}
