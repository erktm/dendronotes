package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMail(t *testing.T) {
	soma, data := dirs(t)
	pw := filepath.Join(t.TempDir(), "smtp")
	if err := os.WriteFile(pw, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := map[string]string{EnvSomasDir: soma, EnvDataDir: data}
	with := func(extra map[string]string) (Config, error) {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return Load(env(m))
	}

	c, err := with(nil)
	if err != nil || c.Mail.Enabled() || c.Mail.DigestDelay != 10*time.Minute {
		t.Fatalf("off by default: %+v %v", c.Mail, err)
	}

	full := map[string]string{
		EnvSMTPHost: "in-v3.mailjet.com", EnvSMTPUsername: "key", EnvSMTPPassword: pw,
		EnvMailFrom: "dendronotes <no-reply@example.org>", EnvPublicURL: "https://notes.example.org",
		EnvMailDigestDelay: "30s",
	}
	c, err = with(full)
	if err != nil {
		t.Fatal(err)
	}
	m := c.Mail
	if !m.Enabled() || m.Port != 587 || m.TLS != "starttls" || m.Password.Reveal() != "s3cret" || m.FromAddr.Address != "no-reply@example.org" ||
		m.DigestDelay != 30*time.Second {
		t.Errorf("parsed: %+v", m)
	}
	if got := c.Describe(EnvSMTPPassword); got != "(set)" {
		t.Errorf("password shown as %q", got)
	}

	for name, tc := range map[string]struct {
		set  map[string]string
		want string
	}{
		"tls":          {map[string]string{EnvSMTPTLS: "ssl"}, EnvSMTPTLS},
		"port":         {map[string]string{EnvSMTPPort: "0"}, EnvSMTPPort},
		"host":         {map[string]string{EnvSMTPHost: "smtp.example.org:587"}, EnvSMTPHost},
		"no password":  {map[string]string{EnvSMTPPassword: ""}, EnvSMTPUsername},
		"no user":      {map[string]string{EnvSMTPUsername: ""}, EnvSMTPPassword},
		"plain auth":   {map[string]string{EnvSMTPTLS: "none"}, EnvSMTPTLS},
		"from":         {map[string]string{EnvMailFrom: "nobody"}, EnvMailFrom},
		"no from":      {map[string]string{EnvMailFrom: ""}, EnvMailFrom},
		"from newline": {map[string]string{EnvMailFrom: "a@example.org\r\nBcc: b@example.org"}, EnvMailFrom},
		"public url":   {map[string]string{EnvPublicURL: ""}, EnvPublicURL},
		"delay":        {map[string]string{EnvMailDigestDelay: "soon"}, EnvMailDigestDelay},
	} {
		m := map[string]string{}
		for k, v := range full {
			m[k] = v
		}
		for k, v := range tc.set {
			m[k] = v
		}
		if _, err := with(m); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want an error about %s, got %v", name, tc.want, err)
		}
	}

	// A relay without a password may go unencrypted (a local one).
	c, err = with(map[string]string{EnvSMTPHost: "localhost", EnvSMTPPort: "25", EnvSMTPTLS: "none",
		EnvMailFrom: "no-reply@example.org", EnvPublicURL: "https://notes.example.org"})
	if err != nil || c.Mail.TLS != "none" || c.Mail.Port != 25 {
		t.Errorf("local relay: %+v %v", c.Mail, err)
	}
}
