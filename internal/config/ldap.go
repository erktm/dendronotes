package config

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Active Directory / LDAP: people sign in with their directory account in the
// normal password form; groups below LDAP_GROUP_BASE_DN become dendronotes
// groups.
const (
	EnvLDAPURL              = Prefix + "LDAP_URL"
	EnvLDAPStartTLS         = Prefix + "LDAP_START_TLS"
	EnvLDAPCAFile           = Prefix + "LDAP_CA_FILE"
	EnvLDAPBindDN           = Prefix + "LDAP_BIND_DN"
	EnvLDAPBindPassword     = Prefix + "LDAP_BIND_PASSWORD_FILE"
	EnvLDAPBaseDN           = Prefix + "LDAP_BASE_DN"
	EnvLDAPUserFilter       = Prefix + "LDAP_USER_FILTER"
	EnvLDAPUsernameAttr     = Prefix + "LDAP_USERNAME_ATTR"
	EnvLDAPNameAttr         = Prefix + "LDAP_NAME_ATTR"
	EnvLDAPEmailAttr        = Prefix + "LDAP_EMAIL_ATTR"
	EnvLDAPIDAttr           = Prefix + "LDAP_ID_ATTR"
	EnvLDAPGroupBaseDN      = Prefix + "LDAP_GROUP_BASE_DN"
	EnvLDAPGroupFilter      = Prefix + "LDAP_GROUP_FILTER"
	EnvLDAPGroupNameAttr    = Prefix + "LDAP_GROUP_NAME_ATTR"
	EnvLDAPAdminGroups      = Prefix + "LDAP_ADMIN_GROUPS"
	EnvLDAPAllowedGroups    = Prefix + "LDAP_ALLOWED_GROUPS"
	EnvLDAPSyncInterval     = Prefix + "LDAP_SYNC_INTERVAL"
	DefaultLDAPSyncInterval = 15 * time.Minute
	DefaultLDAPUserFilter   = "(&(objectCategory=person)(objectClass=user)" +
		"(|(sAMAccountName={login})(userPrincipalName={login})(mail={login})))"
	// The matching rule finds nested groups too (Active Directory).
	DefaultLDAPGroupFilter   = "(&(objectClass=group)(member:1.2.840.113556.1.4.1941:={dn}))"
	DefaultLDAPUsernameAttr  = "sAMAccountName"
	DefaultLDAPNameAttr      = "displayName"
	DefaultLDAPEmailAttr     = "mail"
	DefaultLDAPIDAttr        = "objectGUID"
	DefaultLDAPGroupNameAttr = "cn"
)

var ldapVars = []string{
	EnvLDAPURL, EnvLDAPStartTLS, EnvLDAPCAFile, EnvLDAPBindDN, EnvLDAPBindPassword, EnvLDAPBaseDN,
	EnvLDAPUserFilter, EnvLDAPUsernameAttr, EnvLDAPNameAttr, EnvLDAPEmailAttr, EnvLDAPIDAttr,
	EnvLDAPGroupBaseDN, EnvLDAPGroupFilter, EnvLDAPGroupNameAttr, EnvLDAPAdminGroups, EnvLDAPAllowedGroups,
	EnvLDAPSyncInterval,
}

// LDAP configures sign-in against Active Directory (or another LDAP
// directory). It is enabled when URL is set.
type LDAP struct {
	URL           string // ldaps://dc.example.org, or ldap:// with StartTLS (plain only for localhost)
	StartTLS      bool
	CAFile        string // extra CA certificates (PEM) for the directory's TLS
	CAPEM         []byte
	BindDN        string // service account that searches (read only); empty: anonymous
	BindPassword  Secret
	BaseDN        string // where accounts and groups are searched
	UserFilter    string // finds the account; {login} is the escaped sign-in name
	UsernameAttr  string
	NameAttr      string
	EmailAttr     string
	IDAttr        string // stable id of accounts and groups (objectGUID, entryUUID)
	GroupBaseDN   string // groups below it become dendronotes groups (empty: none)
	GroupFilter   string // finds an account's groups; {dn} is the escaped account DN
	GroupNameAttr string
	AdminGroups   []string // members become admins (names or DNs; empty: roles stay in dendronotes)
	AllowedGroups []string // only members may sign in (empty: every account found)
	// SyncInterval checks all directory accounts in the background (0: off).
	SyncInterval time.Duration
}

// Enabled reports whether directory sign-in is configured.
func (l LDAP) Enabled() bool { return l.URL != "" }

// NeedsGroups reports whether sign-in has to look up the account's groups.
func (l LDAP) NeedsGroups() bool {
	return l.GroupBaseDN != "" || len(l.AdminGroups) > 0 || len(l.AllowedGroups) > 0
}

// loadLDAP reads the directory settings; all of them are ignored unless the
// URL is set.
func (a *Auth) loadLDAP(getenv func(string) string) error {
	l := &a.LDAP
	l.URL = strings.TrimSpace(getenv(EnvLDAPURL))
	if l.URL == "" {
		return nil
	}
	var errs []error
	if v := strings.TrimSpace(getenv(EnvLDAPStartTLS)); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: invalid value %q (true, false)", EnvLDAPStartTLS, v))
		}
		l.StartTLS = b
	}
	u, err := url.Parse(l.URL)
	switch {
	case err != nil || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "":
		errs = append(errs, fmt.Errorf("%s: %q must be ldaps://host[:port] or ldap://host[:port]", EnvLDAPURL, l.URL))
	case u.Scheme == "ldaps":
		if l.StartTLS {
			errs = append(errs, fmt.Errorf("%s: %s is for ldap:// URLs; ldaps:// is encrypted already", EnvLDAPStartTLS, EnvLDAPURL))
		}
	case u.Scheme == "ldap":
		if !l.StartTLS && !loopback(u.Hostname()) {
			errs = append(errs, fmt.Errorf("%s: passwords would travel unencrypted – use ldaps:// or set %s=true", EnvLDAPURL, EnvLDAPStartTLS))
		}
	default:
		errs = append(errs, fmt.Errorf("%s: %q must be ldaps://host[:port] or ldap://host[:port]", EnvLDAPURL, l.URL))
	}
	if a.Mode != "local" {
		errs = append(errs, fmt.Errorf("%s works only with %s=local", EnvLDAPURL, EnvAuthMode))
	}
	if f := getenv(EnvLDAPCAFile); f != "" {
		b, err := os.ReadFile(f)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", EnvLDAPCAFile, err))
		case !x509.NewCertPool().AppendCertsFromPEM(b):
			errs = append(errs, fmt.Errorf("%s: %s contains no PEM certificates", EnvLDAPCAFile, f))
		}
		l.CAFile, l.CAPEM = f, b
	}
	l.BindDN = strings.TrimSpace(getenv(EnvLDAPBindDN))
	if f := getenv(EnvLDAPBindPassword); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvLDAPBindPassword, err))
		}
		l.BindPassword = Secret(strings.TrimRight(string(b), "\r\n"))
	}
	if l.BindDN != "" && l.BindPassword == "" {
		errs = append(errs, fmt.Errorf("%s needs %s (the service account's password)", EnvLDAPBindDN, EnvLDAPBindPassword))
	}
	l.BaseDN = strings.TrimSpace(getenv(EnvLDAPBaseDN))
	if l.BaseDN == "" {
		errs = append(errs, fmt.Errorf("%s is required with %s, e.g. DC=example,DC=org", EnvLDAPBaseDN, EnvLDAPURL))
	}
	l.UserFilter = strings.TrimSpace(envOr(getenv, EnvLDAPUserFilter, DefaultLDAPUserFilter))
	if !strings.Contains(l.UserFilter, "{login}") || !wrapped(l.UserFilter) {
		errs = append(errs, fmt.Errorf("%s: must be an LDAP filter in parentheses with {login}", EnvLDAPUserFilter))
	}
	l.UsernameAttr = strings.TrimSpace(envOr(getenv, EnvLDAPUsernameAttr, DefaultLDAPUsernameAttr))
	l.NameAttr = strings.TrimSpace(envOr(getenv, EnvLDAPNameAttr, DefaultLDAPNameAttr))
	l.EmailAttr = strings.TrimSpace(envOr(getenv, EnvLDAPEmailAttr, DefaultLDAPEmailAttr))
	l.IDAttr = strings.TrimSpace(envOr(getenv, EnvLDAPIDAttr, DefaultLDAPIDAttr))
	l.GroupBaseDN = strings.TrimSpace(getenv(EnvLDAPGroupBaseDN))
	l.GroupFilter = strings.TrimSpace(envOr(getenv, EnvLDAPGroupFilter, DefaultLDAPGroupFilter))
	if !strings.Contains(l.GroupFilter, "{dn}") || !wrapped(l.GroupFilter) {
		errs = append(errs, fmt.Errorf("%s: must be an LDAP filter in parentheses with {dn}", EnvLDAPGroupFilter))
	}
	l.GroupNameAttr = strings.TrimSpace(envOr(getenv, EnvLDAPGroupNameAttr, DefaultLDAPGroupNameAttr))
	l.AdminGroups = splitGroups(getenv(EnvLDAPAdminGroups))
	l.AllowedGroups = splitGroups(getenv(EnvLDAPAllowedGroups))
	l.SyncInterval = DefaultLDAPSyncInterval
	if v := strings.TrimSpace(getenv(EnvLDAPSyncInterval)); v != "" {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil || d < 0:
			errs = append(errs, fmt.Errorf("%s: %q is not a duration (e.g. 15m, or 0 to turn it off)", EnvLDAPSyncInterval, v))
		case d > 0 && d < time.Minute:
			errs = append(errs, fmt.Errorf("%s: %q is too short (minimum 1m)", EnvLDAPSyncInterval, v))
		default:
			l.SyncInterval = d
		}
	}
	return errors.Join(errs...)
}

// splitGroups reads a list of group names or DNs. DNs contain commas, so the
// list is separated by semicolons (or new lines).
func splitGroups(s string) []string {
	var out []string
	for item := range strings.FieldsFuncSeq(s, func(r rune) bool { return r == ';' || r == '\n' }) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func wrapped(filter string) bool {
	return strings.HasPrefix(filter, "(") && strings.HasSuffix(filter, ")")
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ldapSetting is the effective value of a directory variable (admin page).
func (c Config) ldapSetting(name string) (string, bool) {
	l := c.Auth.LDAP
	switch name {
	case EnvLDAPURL:
		return l.URL, true
	case EnvLDAPStartTLS:
		return strconv.FormatBool(l.StartTLS), true
	case EnvLDAPCAFile:
		return l.CAFile, true
	case EnvLDAPBindDN:
		return l.BindDN, true
	case EnvLDAPBindPassword:
		if l.BindPassword != "" {
			return "(set)", true
		}
		return "", true
	case EnvLDAPBaseDN:
		return l.BaseDN, true
	case EnvLDAPUserFilter:
		return l.UserFilter, true
	case EnvLDAPUsernameAttr:
		return l.UsernameAttr, true
	case EnvLDAPNameAttr:
		return l.NameAttr, true
	case EnvLDAPEmailAttr:
		return l.EmailAttr, true
	case EnvLDAPIDAttr:
		return l.IDAttr, true
	case EnvLDAPGroupBaseDN:
		return l.GroupBaseDN, true
	case EnvLDAPGroupFilter:
		return l.GroupFilter, true
	case EnvLDAPGroupNameAttr:
		return l.GroupNameAttr, true
	case EnvLDAPAdminGroups:
		return strings.Join(l.AdminGroups, "; "), true
	case EnvLDAPAllowedGroups:
		return strings.Join(l.AllowedGroups, "; "), true
	case EnvLDAPSyncInterval:
		if !l.Enabled() {
			return "", true
		}
		return l.SyncInterval.String(), true
	}
	return "", false
}
