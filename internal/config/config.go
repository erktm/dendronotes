// Package config reads the configuration from environment variables and validates it.
//
// Every variable carries the DENDRONOTES_ prefix. The listening port is
// DENDRONOTES_HTTP_PORT on purpose: Kubernetes injects DENDRONOTES_PORT and
// DENDRONOTES_SERVICE_* into pods when a Service is named dendronotes.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path"
	"path/filepath"
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
	EnvLogLevel        = Prefix + "LOG_LEVEL"
	EnvLogFormat       = Prefix + "LOG_FORMAT"
	EnvShutdownTimeout = Prefix + "SHUTDOWN_TIMEOUT"
	EnvTrustedProxies  = Prefix + "TRUSTED_PROXIES"
	EnvMaxNoteMB       = Prefix + "MAX_NOTE_MB"
	EnvMaxUploadMB     = Prefix + "MAX_UPLOAD_MB"
	EnvIgnore          = Prefix + "IGNORE"
	EnvRescanInterval  = Prefix + "RESCAN_INTERVAL"
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
)

// Vars lists every supported variable. README.md documents all of them
// (enforced by a test).
var Vars = []string{
	EnvSomasDir, EnvSomaDir, EnvDataDir, EnvHTTPPort, EnvLogLevel, EnvLogFormat, EnvShutdownTimeout,
	EnvTrustedProxies, EnvMaxNoteMB, EnvMaxUploadMB, EnvIgnore, EnvRescanInterval, EnvAuthMode, EnvAdminUsername,
	EnvAdminHash, EnvAdminHashFile, EnvAdminPassword, EnvSessionTTL, EnvCookieSecure,
	EnvProxyHeader, EnvProxyAllowed, EnvProxyLogoutURL,
}

// Config holds all runtime settings.
type Config struct {
	SomasDir        string         // one folder per soma (source of truth)
	ImportSomaDir   string         // an existing soma to register for the first admin (optional)
	DataDir         string         // derived caches (index, sessions)
	Port            int            // HTTP port
	LogLevel        slog.Level     // debug|info|warn|error
	LogFormat       string         // json|text
	ShutdownTimeout time.Duration  // max. time to drain on shutdown
	TrustedProxies  []netip.Prefix // reverse proxies whose X-Forwarded-* headers are believed
	MaxNoteBytes    int64          // largest note the editor opens or saves
	MaxUploadBytes  int64          // largest file that can be uploaded
	Ignore          []string       // extra name patterns hidden from the tree
	RescanInterval  time.Duration  // periodic full comparison of soma and index; 0 = off
	Auth            Auth
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
}

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
	DefaultProxyHeader     = "X-Forwarded-User"
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
		Auth: Auth{
			Mode:          "local",
			AdminUsername: DefaultAdminUsername,
			SessionTTL:    DefaultSessionTTL,
			CookieSecure:  "auto",
			ProxyHeader:   envOr(getenv, EnvProxyHeader, DefaultProxyHeader),
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
	}
	return ""
}
