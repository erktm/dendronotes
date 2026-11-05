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
	EnvSomaDir          = Prefix + "SOMA_DIR"
	EnvDataDir          = Prefix + "DATA_DIR"
	EnvHTTPPort         = Prefix + "HTTP_PORT"
	EnvLogLevel         = Prefix + "LOG_LEVEL"
	EnvLogFormat        = Prefix + "LOG_FORMAT"
	EnvShutdownTimeout  = Prefix + "SHUTDOWN_TIMEOUT"
	EnvTrustedProxies   = Prefix + "TRUSTED_PROXIES"
	EnvMaxNoteMB        = Prefix + "MAX_NOTE_MB"
	EnvIgnore           = Prefix + "IGNORE"
	EnvAuthMode         = Prefix + "AUTH_MODE"
	EnvPasswordHash     = Prefix + "AUTH_PASSWORD_HASH"
	EnvPasswordHashFile = Prefix + "AUTH_PASSWORD_HASH_FILE"
	EnvPasswordFile     = Prefix + "AUTH_PASSWORD_FILE"
	EnvSessionTTL       = Prefix + "SESSION_TTL"
	EnvCookieSecure     = Prefix + "COOKIE_SECURE"
	EnvProxyHeader      = Prefix + "AUTH_PROXY_HEADER"
	EnvProxyAllowed     = Prefix + "AUTH_PROXY_ALLOWED_USERS"
	EnvProxyLogoutURL   = Prefix + "AUTH_PROXY_LOGOUT_URL"
)

// Vars lists every supported variable. README.md documents all of them
// (enforced by a test).
var Vars = []string{
	EnvSomaDir, EnvDataDir, EnvHTTPPort, EnvLogLevel, EnvLogFormat, EnvShutdownTimeout,
	EnvTrustedProxies, EnvMaxNoteMB, EnvIgnore, EnvAuthMode, EnvPasswordHash,
	EnvPasswordHashFile, EnvPasswordFile, EnvSessionTTL, EnvCookieSecure,
	EnvProxyHeader, EnvProxyAllowed, EnvProxyLogoutURL,
}

// Config holds all runtime settings.
type Config struct {
	SomaDir         string         // root of the soma (source of truth)
	DataDir         string         // derived caches (index, sessions)
	Port            int            // HTTP port
	LogLevel        slog.Level     // debug|info|warn|error
	LogFormat       string         // json|text
	ShutdownTimeout time.Duration  // max. time to drain on shutdown
	TrustedProxies  []netip.Prefix // reverse proxies whose X-Forwarded-* headers are believed
	MaxNoteBytes    int64          // largest note the editor opens or saves
	Ignore          []string       // extra name patterns hidden from the tree
	Auth            Auth
}

// Auth holds the authentication settings.
type Auth struct {
	Mode           string        // password|proxy
	PasswordHash   string        // argon2id hash (from the variable or a file)
	Password       Secret        // plain password from a file, hashed at startup
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
	DefaultSomaDir         = "/soma"
	DefaultDataDir         = "/data"
	DefaultPort            = 8080
	DefaultShutdownTimeout = 20 * time.Second
	DefaultSessionTTL      = 30 * 24 * time.Hour
	DefaultMaxNoteMB       = 5
	DefaultProxyHeader     = "X-Forwarded-User"
	minPasswordLen         = 8
)

// Load reads the configuration via getenv (usually os.Getenv) and checks the
// directories. All problems found are returned together.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		SomaDir:         envOr(getenv, EnvSomaDir, DefaultSomaDir),
		DataDir:         envOr(getenv, EnvDataDir, DefaultDataDir),
		Port:            DefaultPort,
		LogLevel:        slog.LevelInfo,
		LogFormat:       "json",
		ShutdownTimeout: DefaultShutdownTimeout,
		MaxNoteBytes:    DefaultMaxNoteMB << 20,
		Auth: Auth{
			Mode:         "password",
			SessionTTL:   DefaultSessionTTL,
			CookieSecure: "auto",
			ProxyHeader:  envOr(getenv, EnvProxyHeader, DefaultProxyHeader),
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

	switch a.Mode {
	case "password":
		sources := 0
		if v := getenv(EnvPasswordHash); v != "" {
			a.PasswordHash = strings.TrimSpace(v)
			sources++
		}
		if f := getenv(EnvPasswordHashFile); f != "" {
			b, err := os.ReadFile(f)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", EnvPasswordHashFile, err))
			}
			a.PasswordHash = strings.TrimSpace(string(b))
			sources++
		}
		if f := getenv(EnvPasswordFile); f != "" {
			b, err := os.ReadFile(f)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", EnvPasswordFile, err))
			}
			pw := strings.TrimRight(string(b), "\r\n")
			if err == nil && len(pw) < minPasswordLen {
				errs = append(errs, fmt.Errorf("%s: the password must have at least %d characters", EnvPasswordFile, minPasswordLen))
			}
			a.Password = Secret(pw)
			sources++
		}
		switch {
		case sources == 0:
			errs = append(errs, fmt.Errorf("%s=password needs one of %s, %s or %s (create a hash with `dendronotes hash-password`)",
				EnvAuthMode, EnvPasswordHash, EnvPasswordHashFile, EnvPasswordFile))
		case sources > 1:
			errs = append(errs, fmt.Errorf("set only one of %s, %s and %s", EnvPasswordHash, EnvPasswordHashFile, EnvPasswordFile))
		}
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
		errs = append(errs, fmt.Errorf("%s: invalid value %q (password, proxy)", EnvAuthMode, a.Mode))
	}
	return errors.Join(errs...)
}

// checkDirs normalizes the soma and data directories and checks that they are usable.
func (c *Config) checkDirs() error {
	var errs []error

	soma, err := absClean(c.SomaDir)
	if err != nil {
		return fmt.Errorf("%s: %w", EnvSomaDir, err)
	}
	c.SomaDir = soma
	if st, err := os.Stat(soma); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvSomaDir, err))
	} else if !st.IsDir() {
		errs = append(errs, fmt.Errorf("%s: %s is not a directory", EnvSomaDir, soma))
	}

	data, err := absClean(c.DataDir)
	if err != nil {
		return errors.Join(append(errs, fmt.Errorf("%s: %w", EnvDataDir, err))...)
	}
	c.DataDir = data
	if err := os.MkdirAll(data, 0o750); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", EnvDataDir, err))
	} else if err := probeWritable(data); err != nil {
		errs = append(errs, fmt.Errorf("%s: %s is not writable: %w", EnvDataDir, data, err))
	}

	// The cache must not live inside the soma: it would be synced along,
	// picked up by the watcher and treated as note content.
	if len(errs) == 0 {
		inside, err := isWithin(soma, data)
		if err != nil {
			errs = append(errs, err)
		} else if inside {
			errs = append(errs, fmt.Errorf("%s (%s) must not be inside %s (%s)", EnvDataDir, data, EnvSomaDir, soma))
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
