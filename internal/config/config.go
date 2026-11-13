// Package config reads the configuration from environment variables and validates it.
//
// Every variable carries the DENDRONOTES_ prefix. The listening port is
// DENDRONOTES_HTTP_PORT on purpose: Kubernetes injects DENDRONOTES_PORT and
// DENDRONOTES_SERVICE_* into pods when a Service is named dendronotes.
package config

import (
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"dendronotes/internal/clientip"
)

// Prefix is shared by all environment variables.
const Prefix = "DENDRONOTES_"

// Names of the environment variables.
const (
	EnvSomasDir        = Prefix + "SOMAS_DIR"
	EnvSomaDir         = Prefix + "SOMA_DIR"
	EnvDataDir         = Prefix + "DATA_DIR"
	EnvHTTPPort        = Prefix + "HTTP_PORT"
	EnvMetricsPort     = Prefix + "METRICS_PORT"
	EnvLogLevel        = Prefix + "LOG_LEVEL"
	EnvLogFormat       = Prefix + "LOG_FORMAT"
	EnvShutdownTimeout = Prefix + "SHUTDOWN_TIMEOUT"
	EnvTrustedProxies  = Prefix + "TRUSTED_PROXIES"
	EnvMaxNoteMB       = Prefix + "MAX_NOTE_MB"
	EnvMaxUploadMB     = Prefix + "MAX_UPLOAD_MB"
	EnvIgnore          = Prefix + "IGNORE"
	EnvRescanInterval  = Prefix + "RESCAN_INTERVAL"
	EnvHistoryDays     = Prefix + "HISTORY_DAYS"
	EnvTrashDays       = Prefix + "TRASH_DAYS"
	EnvAuthMode        = Prefix + "AUTH_MODE"
	EnvAdminUsername   = Prefix + "ADMIN_USERNAME"
	EnvAdminHash       = Prefix + "ADMIN_PASSWORD_HASH"
	EnvAdminHashFile   = Prefix + "ADMIN_PASSWORD_HASH_FILE"
	EnvAdminPassword   = Prefix + "ADMIN_PASSWORD_FILE"
	EnvSessionTTL      = Prefix + "SESSION_TTL"
	EnvCookieSecure    = Prefix + "COOKIE_SECURE"
	EnvProxyHeader     = Prefix + "AUTH_PROXY_HEADER"
	EnvProxyAllowed    = Prefix + "AUTH_PROXY_ALLOWED_USERS"
	EnvProxyLogoutURL  = Prefix + "AUTH_PROXY_LOGOUT_URL"
	EnvPublicURL       = Prefix + "PUBLIC_URL"
	EnvLocalLogin      = Prefix + "LOCAL_LOGIN"
	EnvOIDCIssuer      = Prefix + "OIDC_ISSUER"
	EnvOIDCClientID    = Prefix + "OIDC_CLIENT_ID"
	EnvOIDCSecretFile  = Prefix + "OIDC_CLIENT_SECRET_FILE"
	EnvOIDCScopes      = Prefix + "OIDC_SCOPES"
	EnvOIDCUserClaim   = Prefix + "OIDC_USERNAME_CLAIM"
	EnvOIDCGroupsClaim = Prefix + "OIDC_GROUPS_CLAIM"
	EnvOIDCAdminGroups = Prefix + "OIDC_ADMIN_GROUPS"
	EnvOIDCAllowed     = Prefix + "OIDC_ALLOWED_GROUPS"
	EnvOIDCLabel       = Prefix + "OIDC_LOGIN_LABEL"
	EnvOIDCCAFile      = Prefix + "OIDC_CA_FILE"
)

// Vars lists every supported variable. README.md documents all of them
// (enforced by a test).
var Vars = []string{
	EnvSomasDir, EnvSomaDir, EnvDataDir, EnvHTTPPort, EnvMetricsPort, EnvLogLevel, EnvLogFormat, EnvShutdownTimeout,
	EnvTrustedProxies, EnvMaxNoteMB, EnvMaxUploadMB, EnvIgnore, EnvRescanInterval, EnvHistoryDays, EnvTrashDays, EnvAuthMode, EnvAdminUsername,
	EnvAdminHash, EnvAdminHashFile, EnvAdminPassword, EnvSessionTTL, EnvCookieSecure,
	EnvProxyHeader, EnvProxyAllowed, EnvProxyLogoutURL, EnvPublicURL, EnvLocalLogin,
	EnvOIDCIssuer, EnvOIDCClientID, EnvOIDCSecretFile, EnvOIDCScopes, EnvOIDCUserClaim, EnvOIDCGroupsClaim,
	EnvOIDCAdminGroups, EnvOIDCAllowed, EnvOIDCLabel, EnvOIDCCAFile,
}

func init() { Vars = append(Vars, ldapVars...) }

// Config holds all runtime settings.
type Config struct {
	SomasDir        string         // one folder per soma (source of truth)
	ImportSomaDir   string         // an existing soma to register for the first admin (optional)
	DataDir         string         // state.db (accounts, sessions) and the search indexes
	Port            int            // HTTP port
	MetricsPort     int            // separate port for /metrics; 0 = off
	LogLevel        slog.Level     // debug|info|warn|error
	LogFormat       string         // json|text
	ShutdownTimeout time.Duration  // max. time to drain on shutdown
	TrustedProxies  []netip.Prefix // reverse proxies whose X-Forwarded-* headers are believed
	MaxNoteBytes    int64          // largest note the editor opens or saves
	MaxUploadBytes  int64          // largest file that can be uploaded
	Ignore          []string       // extra name patterns hidden from the tree
	RescanInterval  time.Duration  // periodic full comparison of soma and index; 0 = off
	HistoryDays     int            // how long earlier versions of notes are kept; 0 = forever
	TrashDays       int            // entries in a soma's trash are deleted for good after this; 0 = never
	Auth            Auth
	Mail            Mail   // email through an SMTP relay (mail.go)
	Signup          Signup // people creating their own accounts (signup.go)
}

// Auth holds the authentication settings.
type Auth struct {
	Mode           string        // local|proxy
	AdminUsername  string        // the first admin account, created if no admin exists
	AdminHash      string        // its argon2id hash (from the variable or a file)
	AdminPassword  Secret        // or its plain password from a file, hashed at startup
	SessionTTL     time.Duration // sliding session lifetime
	CookieSecure   string        // auto|true|false
	ProxyHeader    string        // header carrying the user in proxy mode
	ProxyAllowed   []string      // optional allow list in proxy mode
	ProxyLogoutURL string        // where "Sign out" leads in proxy mode
	PublicURL      string        // external base URL, e.g. https://notes.example.org (no trailing slash)
	LocalLogin     bool          // sign-in with user name and password for everyone (else only the admin)
	OIDC           OIDC          // sign-in through an OpenID Connect provider (Keycloak)
	LDAP           LDAP          // sign-in against Active Directory / LDAP (ldap.go)
}

// OIDC configures sign-in through an OpenID Connect provider. It is enabled
// when Issuer is set.
type OIDC struct {
	Issuer        string   // e.g. https://sso.example.org/realms/main
	ClientID      string   // the client registered for dendronotes
	ClientSecret  Secret   // from a file; empty for a public client (PKCE only)
	Scopes        []string // always includes openid
	UsernameClaim string   // claim that becomes the user name
	GroupsClaim   string   // claim with the groups (dotted path, e.g. realm_access.roles)
	AdminGroups   []string // members become admins (empty: roles are managed in dendronotes)
	AllowedGroups []string // only members may sign in (empty: everyone)
	LoginLabel    string   // name of the provider on the sign-in button
	CAFile        string   // extra CA certificates (PEM) for the provider's TLS
	CAPEM         []byte   // contents of CAFile
}

// Enabled reports whether OIDC sign-in is configured.
func (o OIDC) Enabled() bool { return o.Issuer != "" }

// Secret is a string that never shows up in logs or formatted output.
type Secret string

func (Secret) String() string                 { return "[redacted]" }
func (Secret) GoString() string               { return "[redacted]" }
func (Secret) LogValue() slog.Value           { return slog.StringValue("[redacted]") }
func (s Secret) Reveal() string               { return string(s) }
func (s Secret) MarshalText() ([]byte, error) { return []byte("[redacted]"), nil }

// Defaults
const (
	DefaultSomasDir        = "/somas"
	DefaultAdminUsername   = "admin"
	DefaultDataDir         = "/data"
	DefaultPort            = 8080
	DefaultShutdownTimeout = 20 * time.Second
	DefaultRescanInterval  = 5 * time.Minute
	DefaultSessionTTL      = 30 * 24 * time.Hour
	DefaultMaxNoteMB       = 5
	DefaultMaxUploadMB     = 25
	DefaultHistoryDays     = 90
	DefaultTrashDays       = 7
	DefaultProxyHeader     = "X-Forwarded-User"
	DefaultOIDCScopes      = "openid profile email"
	DefaultOIDCUserClaim   = "preferred_username"
	DefaultOIDCGroupsClaim = "groups"
	DefaultOIDCLabel       = "SSO"
	minPasswordLen         = 8
)

// Load reads the configuration via getenv (usually os.Getenv) and checks the
// directories. All problems found are returned together.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		SomasDir:        envOr(getenv, EnvSomasDir, DefaultSomasDir),
		ImportSomaDir:   getenv(EnvSomaDir),
		DataDir:         envOr(getenv, EnvDataDir, DefaultDataDir),
		Port:            DefaultPort,
		LogLevel:        slog.LevelInfo,
		LogFormat:       "json",
		ShutdownTimeout: DefaultShutdownTimeout,
		RescanInterval:  DefaultRescanInterval,
		MaxNoteBytes:    DefaultMaxNoteMB << 20,
		MaxUploadBytes:  DefaultMaxUploadMB << 20,
		HistoryDays:     DefaultHistoryDays,
		TrashDays:       DefaultTrashDays,
		Auth: Auth{
			Mode:          "local",
			AdminUsername: DefaultAdminUsername,
			SessionTTL:    DefaultSessionTTL,
			CookieSecure:  "auto",
			ProxyHeader:   envOr(getenv, EnvProxyHeader, DefaultProxyHeader),
			LocalLogin:    true,
		},
	}
	var errs []error
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if v := getenv(EnvHTTPPort); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			add(fmt.Errorf("%s: %q is not a valid port (1-65535)", EnvHTTPPort, v))
		} else {
			c.Port = p
		}
	}

	if v := getenv(EnvMetricsPort); v != "" {
		p, err := strconv.Atoi(v)
		switch {
		case err != nil || p < 1 || p > 65535:
			add(fmt.Errorf("%s: %q is not a valid port (1-65535)", EnvMetricsPort, v))
		case p == c.Port:
			add(fmt.Errorf("%s: %d is the HTTP port already; metrics need a port of their own", EnvMetricsPort, p))
		default:
			c.MetricsPort = p
		}
	}

	if v := getenv(EnvLogLevel); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(v)); err != nil {
			add(fmt.Errorf("%s: invalid value %q (debug, info, warn, error)", EnvLogLevel, v))
		}
	}

	if v := strings.ToLower(getenv(EnvLogFormat)); v != "" {
		if v != "json" && v != "text" {
			add(fmt.Errorf("%s: invalid value %q (json, text)", EnvLogFormat, v))
		} else {
			c.LogFormat = v
		}
	}

	if v := getenv(EnvShutdownTimeout); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			add(fmt.Errorf("%s: %q is not a positive duration (e.g. 20s)", EnvShutdownTimeout, v))
		} else {
			c.ShutdownTimeout = d
		}
	}

	if v := getenv(EnvRescanInterval); v != "" {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil || d < 0:
			add(fmt.Errorf("%s: %q is not a duration (e.g. 5m, or 0 to disable)", EnvRescanInterval, v))
		case d > 0 && d < 10*time.Second:
			add(fmt.Errorf("%s: %q is too short (minimum 10s)", EnvRescanInterval, v))
		default:
			c.RescanInterval = d
		}
	}

	if v := getenv(EnvTrustedProxies); v != "" {
		p, err := clientip.ParseList(v)
		if err != nil {
			add(fmt.Errorf("%s: %v (expected IPs or CIDRs, comma-separated)", EnvTrustedProxies, err))
		}
		c.TrustedProxies = p
	}

	if v := getenv(EnvHistoryDays); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil || days < 0 || days > 36500 {
			add(fmt.Errorf("%s: %q must be a whole number of days (0 keeps everything)", EnvHistoryDays, v))
		} else {
			c.HistoryDays = days
		}
	}

	if v := getenv(EnvTrashDays); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil || days < 0 || days > 36500 {
			add(fmt.Errorf("%s: %q must be a whole number of days (0 keeps everything)", EnvTrashDays, v))
		} else {
			c.TrashDays = days
		}
	}

	if v := getenv(EnvMaxNoteMB); v != "" {
		mb, err := strconv.Atoi(v)
		if err != nil || mb < 1 || mb > 100 {
			add(fmt.Errorf("%s: %q must be a whole number between 1 and 100", EnvMaxNoteMB, v))
		} else {
			c.MaxNoteBytes = int64(mb) << 20
		}
	}

	if v := getenv(EnvMaxUploadMB); v != "" {
		mb, err := strconv.Atoi(v)
		if err != nil || mb < 1 || mb > 4096 {
			add(fmt.Errorf("%s: %q must be a whole number between 1 and 4096", EnvMaxUploadMB, v))
		} else {
			c.MaxUploadBytes = int64(mb) << 20
		}
	}

	for _, p := range splitList(getenv(EnvIgnore)) {
		if _, err := path.Match(p, ""); err != nil || strings.Contains(p, "/") {
			add(fmt.Errorf("%s: invalid name pattern %q", EnvIgnore, p))
			continue
		}
		c.Ignore = append(c.Ignore, p)
	}

	add(c.loadAuth(getenv))
	add(c.loadMail(getenv))   // after auth: the links need the public URL
	add(c.loadSignup(getenv)) // after mail and auth: it needs both
	add(c.checkDirs())
	return c, errors.Join(errs...)
}

func (c *Config) loadAuth(getenv func(string) string) error {
	var errs []error
	a := &c.Auth

	if v := strings.ToLower(getenv(EnvAuthMode)); v != "" {
		a.Mode = v
	}
	if v := getenv(EnvSessionTTL); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 5*time.Minute {
			errs = append(errs, fmt.Errorf("%s: %q must be a duration of at least 5m (e.g. 720h)", EnvSessionTTL, v))
		} else {
			a.SessionTTL = d
		}
	}
	if v := strings.ToLower(getenv(EnvCookieSecure)); v != "" {
		if v != "auto" && v != "true" && v != "false" {
			errs = append(errs, fmt.Errorf("%s: invalid value %q (auto, true, false)", EnvCookieSecure, v))
		} else {
			a.CookieSecure = v
		}
	}

	if v := strings.TrimSpace(getenv(EnvAdminUsername)); v != "" {
		a.AdminUsername = v
	}
	sources := 0
	if v := getenv(EnvAdminHash); v != "" {
		a.AdminHash = strings.TrimSpace(v)
		sources++
	}
	if f := getenv(EnvAdminHashFile); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvAdminHashFile, err))
		}
		a.AdminHash = strings.TrimSpace(string(b))
		sources++
	}
	if f := getenv(EnvAdminPassword); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvAdminPassword, err))
		}
		pw := strings.TrimRight(string(b), "\r\n")
		if err == nil && len(pw) < minPasswordLen {
			errs = append(errs, fmt.Errorf("%s: the password must have at least %d characters", EnvAdminPassword, minPasswordLen))
		}
		a.AdminPassword = Secret(pw)
		sources++
	}
	if sources > 1 {
		errs = append(errs, fmt.Errorf("set only one of %s, %s and %s", EnvAdminHash, EnvAdminHashFile, EnvAdminPassword))
	}

	if v := strings.TrimSpace(getenv(EnvPublicURL)); v != "" {
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil ||
			u.RawQuery != "" || u.Fragment != "" {
			errs = append(errs, fmt.Errorf("%s: %q must be an http(s) URL without query, e.g. https://notes.example.org", EnvPublicURL, v))
		} else {
			a.PublicURL = strings.TrimRight(u.String(), "/")
		}
	}
	if v := strings.ToLower(strings.TrimSpace(getenv(EnvLocalLogin))); v != "" {
		switch v {
		case "true":
		case "false":
			a.LocalLogin = false
		default:
			errs = append(errs, fmt.Errorf("%s: invalid value %q (true, false)", EnvLocalLogin, v))
		}
	}
	errs = append(errs, a.loadOIDC(getenv))

	switch a.Mode {
	case "local":
	case "proxy":
		if len(c.TrustedProxies) == 0 {
			errs = append(errs, fmt.Errorf("%s=proxy needs %s, otherwise anyone could send the user header", EnvAuthMode, EnvTrustedProxies))
		}
		a.ProxyAllowed = splitList(getenv(EnvProxyAllowed))
		if v := getenv(EnvProxyLogoutURL); v != "" {
			if !validLogoutURL(v) {
				errs = append(errs, fmt.Errorf("%s: %q must be a path starting with / or an http(s) URL", EnvProxyLogoutURL, v))
			} else {
				a.ProxyLogoutURL = v
			}
		}
	default:
		errs = append(errs, fmt.Errorf("%s: invalid value %q (local, proxy)", EnvAuthMode, a.Mode))
	}
	if a.OIDC.Enabled() && a.Mode != "local" {
		errs = append(errs, fmt.Errorf("%s works only with %s=local (in proxy mode the proxy signs people in)", EnvOIDCIssuer, EnvAuthMode))
	}
	errs = append(errs, a.loadLDAP(getenv)) // after the mode: LDAP needs local
	if !a.LocalLogin && !a.OIDC.Enabled() && !a.LDAP.Enabled() {
		errs = append(errs, fmt.Errorf("%s=false needs %s or %s, otherwise only the admin could sign in", EnvLocalLogin, EnvOIDCIssuer, EnvLDAPURL))
	}
	return errors.Join(errs...)
}

// loadOIDC reads the OpenID Connect settings; all of them are ignored unless
// the issuer is set.
func (a *Auth) loadOIDC(getenv func(string) string) error {
	o := &a.OIDC
	o.Issuer = strings.TrimRight(strings.TrimSpace(getenv(EnvOIDCIssuer)), "/")
	if o.Issuer == "" {
		return nil
	}
	var errs []error
	if u, err := url.Parse(o.Issuer); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
		u.RawQuery != "" || u.Fragment != "" {
		errs = append(errs, fmt.Errorf("%s: %q must be the issuer URL, e.g. https://sso.example.org/realms/main", EnvOIDCIssuer, o.Issuer))
	}
	o.ClientID = strings.TrimSpace(getenv(EnvOIDCClientID))
	if o.ClientID == "" {
		errs = append(errs, fmt.Errorf("%s is required with %s", EnvOIDCClientID, EnvOIDCIssuer))
	}
	if a.PublicURL == "" {
		errs = append(errs, fmt.Errorf("%s is required with %s (it gives the redirect URI)", EnvPublicURL, EnvOIDCIssuer))
	}
	if f := getenv(EnvOIDCSecretFile); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvOIDCSecretFile, err))
		}
		o.ClientSecret = Secret(strings.TrimSpace(string(b)))
	}
	o.Scopes = strings.Fields(strings.ReplaceAll(envOr(getenv, EnvOIDCScopes, DefaultOIDCScopes), ",", " "))
	if !slices.Contains(o.Scopes, "openid") {
		o.Scopes = append([]string{"openid"}, o.Scopes...)
	}
	o.UsernameClaim = strings.TrimSpace(envOr(getenv, EnvOIDCUserClaim, DefaultOIDCUserClaim))
	o.GroupsClaim = strings.TrimSpace(envOr(getenv, EnvOIDCGroupsClaim, DefaultOIDCGroupsClaim))
	o.AdminGroups = splitList(getenv(EnvOIDCAdminGroups))
	o.AllowedGroups = splitList(getenv(EnvOIDCAllowed))
	o.LoginLabel = strings.TrimSpace(envOr(getenv, EnvOIDCLabel, DefaultOIDCLabel))
	if len(o.LoginLabel) > 40 {
		errs = append(errs, fmt.Errorf("%s: at most 40 characters", EnvOIDCLabel))
	}
	if f := getenv(EnvOIDCCAFile); f != "" {
		b, err := os.ReadFile(f)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", EnvOIDCCAFile, err))
		case !x509.NewCertPool().AppendCertsFromPEM(b):
			errs = append(errs, fmt.Errorf("%s: %s contains no PEM certificates", EnvOIDCCAFile, f))
		}
		o.CAFile, o.CAPEM = f, b
	}
	return errors.Join(errs...)
}

// checkDirs normalizes the somas and data directories and checks that they are usable.
func (c *Config) checkDirs() error {
	var errs []error
	usable := func(env, dir string) (string, error) {
		abs, err := absClean(dir)
		if err != nil {
			return "", fmt.Errorf("%s: %w", env, err)
		}
		if err := os.MkdirAll(abs, 0o750); err != nil {
			return abs, fmt.Errorf("%s: %w", env, err)
		}
		if err := probeWritable(abs); err != nil {
			return abs, fmt.Errorf("%s: %s is not writable: %w", env, abs, err)
		}
		return abs, nil
	}

	somas, err := usable(EnvSomasDir, c.SomasDir)
	c.SomasDir = somas
	if err != nil {
		errs = append(errs, err)
	}
	data, err := usable(EnvDataDir, c.DataDir)
	c.DataDir = data
	if err != nil {
		errs = append(errs, err)
	}
	if c.ImportSomaDir != "" {
		imp, err := absClean(c.ImportSomaDir)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvSomaDir, err))
		} else if st, err := os.Stat(imp); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", EnvSomaDir, err))
		} else if !st.IsDir() {
			errs = append(errs, fmt.Errorf("%s: %s is not a directory", EnvSomaDir, imp))
		}
		c.ImportSomaDir = imp
	}

	// The data must not live inside a soma: it would be synced along, picked
	// up by the watcher and treated as note content.
	if len(errs) == 0 {
		roots := map[string]string{EnvSomasDir: somas}
		if c.ImportSomaDir != "" {
			roots[EnvSomaDir] = c.ImportSomaDir
		}
		for env, root := range roots {
			inside, err := isWithin(root, data)
			if err != nil {
				errs = append(errs, err)
			} else if inside {
				errs = append(errs, fmt.Errorf("%s (%s) must not be inside %s (%s)", EnvDataDir, data, env, root))
			}
		}
	}
	return errors.Join(errs...)
}

func envOr(getenv func(string) string, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

// validLogoutURL allows only same-site paths and http(s) URLs, because the
// browser navigates there (no javascript: or protocol-relative URLs).
func validLogoutURL(v string) bool {
	if strings.HasPrefix(v, "//") || strings.ContainsAny(v, " \t\r\n") {
		return false
	}
	return strings.HasPrefix(v, "/") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://")
}

func splitList(s string) []string {
	var out []string
	for item := range strings.SplitSeq(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func absClean(p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", errors.New("path contains NUL")
	}
	return filepath.Abs(p)
}

func probeWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".dendronotes-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// isWithin reports whether child equals parent or lies below it
// (after resolving symlinks on both sides).
func isWithin(parent, child string) (bool, error) {
	p, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return false, err
	}
	c, err := filepath.EvalSymlinks(child)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(p, c)
	if err != nil {
		return false, nil
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

// Describe returns the effective value of a variable for display (admin
// area). Secrets are never shown, only whether they are set.
func (c Config) Describe(name string) string {
	secret := func(set bool) string {
		if set {
			return "(set)"
		}
		return ""
	}
	switch name {
	case EnvSomasDir:
		return c.SomasDir
	case EnvSomaDir:
		return c.ImportSomaDir
	case EnvDataDir:
		return c.DataDir
	case EnvMetricsPort:
		if c.MetricsPort == 0 {
			return ""
		}
		return strconv.Itoa(c.MetricsPort)
	case EnvHTTPPort:
		return strconv.Itoa(c.Port)
	case EnvLogLevel:
		return strings.ToLower(c.LogLevel.String())
	case EnvLogFormat:
		return c.LogFormat
	case EnvShutdownTimeout:
		return c.ShutdownTimeout.String()
	case EnvTrustedProxies:
		parts := make([]string, len(c.TrustedProxies))
		for i, p := range c.TrustedProxies {
			parts[i] = p.String()
		}
		return strings.Join(parts, ", ")
	case EnvMaxNoteMB:
		return strconv.FormatInt(c.MaxNoteBytes>>20, 10)
	case EnvMaxUploadMB:
		return strconv.FormatInt(c.MaxUploadBytes>>20, 10)
	case EnvIgnore:
		return strings.Join(c.Ignore, ", ")
	case EnvRescanInterval:
		return c.RescanInterval.String()
	case EnvHistoryDays:
		return strconv.Itoa(c.HistoryDays)
	case EnvTrashDays:
		return strconv.Itoa(c.TrashDays)
	case EnvAuthMode:
		return c.Auth.Mode
	case EnvAdminUsername:
		return c.Auth.AdminUsername
	case EnvAdminHash, EnvAdminHashFile:
		return secret(c.Auth.AdminHash != "")
	case EnvAdminPassword:
		return secret(c.Auth.AdminPassword != "")
	case EnvSessionTTL:
		return c.Auth.SessionTTL.String()
	case EnvCookieSecure:
		return c.Auth.CookieSecure
	case EnvProxyHeader:
		return c.Auth.ProxyHeader
	case EnvProxyAllowed:
		return strings.Join(c.Auth.ProxyAllowed, ", ")
	case EnvProxyLogoutURL:
		return c.Auth.ProxyLogoutURL
	case EnvPublicURL:
		return c.Auth.PublicURL
	case EnvLocalLogin:
		return strconv.FormatBool(c.Auth.LocalLogin)
	case EnvOIDCIssuer:
		return c.Auth.OIDC.Issuer
	case EnvOIDCClientID:
		return c.Auth.OIDC.ClientID
	case EnvOIDCSecretFile:
		return secret(c.Auth.OIDC.ClientSecret != "")
	case EnvOIDCScopes:
		return strings.Join(c.Auth.OIDC.Scopes, " ")
	case EnvOIDCUserClaim:
		return c.Auth.OIDC.UsernameClaim
	case EnvOIDCGroupsClaim:
		return c.Auth.OIDC.GroupsClaim
	case EnvOIDCAdminGroups:
		return strings.Join(c.Auth.OIDC.AdminGroups, ", ")
	case EnvOIDCAllowed:
		return strings.Join(c.Auth.OIDC.AllowedGroups, ", ")
	case EnvOIDCLabel:
		return c.Auth.OIDC.LoginLabel
	case EnvOIDCCAFile:
		return c.Auth.OIDC.CAFile
	}
	if v, ok := c.ldapSetting(name); ok {
		return v
	}
	if v, ok := c.mailSetting(name); ok {
		return v
	}
	if v, ok := c.signupSetting(name); ok {
		return v
	}
	return ""
}
