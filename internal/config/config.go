// Package config reads the configuration from environment variables and validates it.
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

	"leafnotes/internal/clientip"
)

// Config holds all runtime settings.
type Config struct {
	VaultDir        string         // VAULT_DIR: root of the vault (source of truth)
	DataDir         string         // DATA_DIR: derived caches (index, sessions)
	Port            int            // PORT
	LogLevel        slog.Level     // LOG_LEVEL: debug|info|warn|error
	LogFormat       string         // LOG_FORMAT: json|text
	ShutdownTimeout time.Duration  // SHUTDOWN_TIMEOUT: max. time to drain on shutdown
	TrustedProxies  []netip.Prefix // TRUSTED_PROXIES: reverse proxies whose X-Forwarded-* headers are believed
	MaxNoteBytes    int64          // MAX_NOTE_MB: largest note the editor opens or saves
	Ignore          []string       // IGNORE: extra name patterns hidden from the tree
	Auth            Auth
}

// Auth holds the authentication settings.
type Auth struct {
	Mode           string        // AUTH_MODE: password|proxy
	PasswordHash   string        // AUTH_PASSWORD_HASH or AUTH_PASSWORD_HASH_FILE (argon2id)
	Password       Secret        // AUTH_PASSWORD_FILE: plaintext, hashed at startup
	SessionTTL     time.Duration // SESSION_TTL: sliding session lifetime
	CookieSecure   string        // COOKIE_SECURE: auto|true|false
	ProxyHeader    string        // AUTH_PROXY_HEADER
	ProxyAllowed   []string      // AUTH_PROXY_ALLOWED_USERS
	ProxyLogoutURL string        // AUTH_PROXY_LOGOUT_URL
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
	DefaultVaultDir        = "/vault"
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
		VaultDir:        envOr(getenv, "VAULT_DIR", DefaultVaultDir),
		DataDir:         envOr(getenv, "DATA_DIR", DefaultDataDir),
		Port:            DefaultPort,
		LogLevel:        slog.LevelInfo,
		LogFormat:       "json",
		ShutdownTimeout: DefaultShutdownTimeout,
		MaxNoteBytes:    DefaultMaxNoteMB << 20,
		Auth: Auth{
			Mode:         "password",
			SessionTTL:   DefaultSessionTTL,
			CookieSecure: "auto",
			ProxyHeader:  envOr(getenv, "AUTH_PROXY_HEADER", DefaultProxyHeader),
		},
	}
	var errs []error
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if v := getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			add(fmt.Errorf("PORT: %q is not a valid port (1-65535)", v))
		} else {
			c.Port = p
		}
	}

	if v := getenv("LOG_LEVEL"); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(v)); err != nil {
			add(fmt.Errorf("LOG_LEVEL: invalid value %q (debug, info, warn, error)", v))
		}
	}

	if v := strings.ToLower(getenv("LOG_FORMAT")); v != "" {
		if v != "json" && v != "text" {
			add(fmt.Errorf("LOG_FORMAT: invalid value %q (json, text)", v))
		} else {
			c.LogFormat = v
		}
	}

	if v := getenv("SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			add(fmt.Errorf("SHUTDOWN_TIMEOUT: %q is not a positive duration (e.g. 20s)", v))
		} else {
			c.ShutdownTimeout = d
		}
	}

	if v := getenv("TRUSTED_PROXIES"); v != "" {
		p, err := clientip.ParseList(v)
		if err != nil {
			add(fmt.Errorf("TRUSTED_PROXIES: %v (expected IPs or CIDRs, comma-separated)", err))
		}
		c.TrustedProxies = p
	}

	if v := getenv("MAX_NOTE_MB"); v != "" {
		mb, err := strconv.Atoi(v)
		if err != nil || mb < 1 || mb > 100 {
			add(fmt.Errorf("MAX_NOTE_MB: %q must be a whole number between 1 and 100", v))
		} else {
			c.MaxNoteBytes = int64(mb) << 20
		}
	}

	for _, p := range splitList(getenv("IGNORE")) {
		if _, err := path.Match(p, ""); err != nil || strings.Contains(p, "/") {
			add(fmt.Errorf("IGNORE: invalid name pattern %q", p))
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

	if v := strings.ToLower(getenv("AUTH_MODE")); v != "" {
		a.Mode = v
	}
	if v := getenv("SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 5*time.Minute {
			errs = append(errs, fmt.Errorf("SESSION_TTL: %q must be a duration of at least 5m (e.g. 720h)", v))
		} else {
			a.SessionTTL = d
		}
	}
	if v := strings.ToLower(getenv("COOKIE_SECURE")); v != "" {
		if v != "auto" && v != "true" && v != "false" {
			errs = append(errs, fmt.Errorf("COOKIE_SECURE: invalid value %q (auto, true, false)", v))
		} else {
			a.CookieSecure = v
		}
	}

	switch a.Mode {
	case "password":
		sources := 0
		if v := getenv("AUTH_PASSWORD_HASH"); v != "" {
			a.PasswordHash = strings.TrimSpace(v)
			sources++
		}
		if f := getenv("AUTH_PASSWORD_HASH_FILE"); f != "" {
			b, err := os.ReadFile(f)
			if err != nil {
				errs = append(errs, fmt.Errorf("AUTH_PASSWORD_HASH_FILE: %w", err))
			}
			a.PasswordHash = strings.TrimSpace(string(b))
			sources++
		}
		if f := getenv("AUTH_PASSWORD_FILE"); f != "" {
			b, err := os.ReadFile(f)
			if err != nil {
				errs = append(errs, fmt.Errorf("AUTH_PASSWORD_FILE: %w", err))
			}
			pw := strings.TrimRight(string(b), "\r\n")
			if err == nil && len(pw) < minPasswordLen {
				errs = append(errs, fmt.Errorf("AUTH_PASSWORD_FILE: the password must have at least %d characters", minPasswordLen))
			}
			a.Password = Secret(pw)
			sources++
		}
		switch {
		case sources == 0:
			errs = append(errs, errors.New("AUTH_MODE=password needs one of AUTH_PASSWORD_HASH, AUTH_PASSWORD_HASH_FILE or AUTH_PASSWORD_FILE (create a hash with `leafnotes hash-password`)"))
		case sources > 1:
			errs = append(errs, errors.New("set only one of AUTH_PASSWORD_HASH, AUTH_PASSWORD_HASH_FILE and AUTH_PASSWORD_FILE"))
		}
	case "proxy":
		if len(c.TrustedProxies) == 0 {
			errs = append(errs, errors.New("AUTH_MODE=proxy needs TRUSTED_PROXIES, otherwise anyone could send the user header"))
		}
		a.ProxyAllowed = splitList(getenv("AUTH_PROXY_ALLOWED_USERS"))
		if v := getenv("AUTH_PROXY_LOGOUT_URL"); v != "" {
			if !validLogoutURL(v) {
				errs = append(errs, fmt.Errorf("AUTH_PROXY_LOGOUT_URL: %q must be a path starting with / or an http(s) URL", v))
			} else {
				a.ProxyLogoutURL = v
			}
		}
	default:
		errs = append(errs, fmt.Errorf("AUTH_MODE: invalid value %q (password, proxy)", a.Mode))
	}
	return errors.Join(errs...)
}

// checkDirs normalizes VAULT_DIR and DATA_DIR and checks that they are usable.
func (c *Config) checkDirs() error {
	var errs []error

	vault, err := absClean(c.VaultDir)
	if err != nil {
		return fmt.Errorf("VAULT_DIR: %w", err)
	}
	c.VaultDir = vault
	if st, err := os.Stat(vault); err != nil {
		errs = append(errs, fmt.Errorf("VAULT_DIR: %w", err))
	} else if !st.IsDir() {
		errs = append(errs, fmt.Errorf("VAULT_DIR: %s is not a directory", vault))
	}

	data, err := absClean(c.DataDir)
	if err != nil {
		return errors.Join(append(errs, fmt.Errorf("DATA_DIR: %w", err))...)
	}
	c.DataDir = data
	if err := os.MkdirAll(data, 0o750); err != nil {
		errs = append(errs, fmt.Errorf("DATA_DIR: %w", err))
	} else if err := probeWritable(data); err != nil {
		errs = append(errs, fmt.Errorf("DATA_DIR: %s is not writable: %w", data, err))
	}

	// The cache must not live inside the vault: it would be synced along,
	// picked up by the watcher and treated as note content.
	if len(errs) == 0 {
		inside, err := isWithin(vault, data)
		if err != nil {
			errs = append(errs, err)
		} else if inside {
			errs = append(errs, fmt.Errorf("DATA_DIR (%s) must not be inside VAULT_DIR (%s)", data, vault))
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
	f, err := os.CreateTemp(dir, ".leafnotes-probe-*")
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
