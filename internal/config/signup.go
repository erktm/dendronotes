package config

import (
	"errors"
	"fmt"
	"strings"
)

// Sign-up: people create their own accounts, confirmed by e-mail. Off unless
// SIGNUP=true; it needs e-mail and password sign-in for everyone.
const (
	EnvSignup        = Prefix + "SIGNUP"
	EnvSignupDomains = Prefix + "SIGNUP_DOMAINS"
)

var signupVars = []string{EnvSignup, EnvSignupDomains}

func init() { Vars = append(Vars, signupVars...) }

// Signup configures people creating their own accounts.
type Signup struct {
	Enabled bool
	// Domains are the e-mail domains allowed to sign up (lower case); empty
	// means every address.
	Domains []string
}

// Allows reports whether an e-mail address may sign up.
func (s Signup) Allows(email string) bool {
	if len(s.Domains) == 0 {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at+1:])
	for _, d := range s.Domains {
		if d == domain {
			return true
		}
	}
	return false
}

// loadSignup reads the sign-up settings; they need e-mail and local passwords.
func (c *Config) loadSignup(getenv func(string) string) error {
	var errs []error
	s := &c.Signup
	switch v := strings.ToLower(strings.TrimSpace(getenv(EnvSignup))); v {
	case "", "false":
	case "true":
		s.Enabled = true
	default:
		errs = append(errs, fmt.Errorf("%s: invalid value %q (true, false)", EnvSignup, v))
	}
	seen := map[string]bool{}
	for _, d := range splitList(strings.ReplaceAll(getenv(EnvSignupDomains), ";", ",")) {
		d = strings.ToLower(strings.TrimPrefix(d, "@"))
		if len(d) > 253 || !strings.Contains(d, ".") || strings.ContainsAny(d, " @/\\\t\r\n<>") {
			errs = append(errs, fmt.Errorf("%s: %q is not a domain, e.g. example.org", EnvSignupDomains, d))
			continue
		}
		if !seen[d] {
			seen[d] = true
			s.Domains = append(s.Domains, d)
		}
	}
	if len(s.Domains) > 0 && !s.Enabled {
		errs = append(errs, fmt.Errorf("%s works only with %s=true", EnvSignupDomains, EnvSignup))
	}
	if s.Enabled {
		if !c.Mail.Enabled() {
			errs = append(errs, fmt.Errorf("%s=true needs e-mail (%s): accounts are confirmed by e-mail", EnvSignup, EnvSMTPHost))
		}
		if c.Auth.Mode != "local" || !c.Auth.LocalLogin {
			errs = append(errs, fmt.Errorf("%s=true needs password sign-in for everyone (%s=local, %s=true)", EnvSignup, EnvAuthMode, EnvLocalLogin))
		}
	}
	return errors.Join(errs...)
}

// signupSetting is the effective value of a sign-up variable (admin page).
func (c Config) signupSetting(name string) (string, bool) {
	switch name {
	case EnvSignup:
		if c.Signup.Enabled {
			return "true", true
		}
		return "false", true
	case EnvSignupDomains:
		return strings.Join(c.Signup.Domains, ", "), true
	}
	return "", false
}
