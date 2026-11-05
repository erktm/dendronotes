package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// env builds a getenv function; a password hash is set unless overridden,
// because every valid configuration needs authentication.
func env(m map[string]string) func(string) string {
	if _, ok := m["AUTH_MODE"]; !ok {
		if _, ok := m["AUTH_PASSWORD_FILE"]; !ok {
			if _, ok := m["AUTH_PASSWORD_HASH_FILE"]; !ok {
				if _, ok := m["AUTH_PASSWORD_HASH"]; !ok {
					m["AUTH_PASSWORD_HASH"] = "$argon2id$placeholder"
				}
			}
		}
	}
	return func(k string) string { return m[k] }
}

func dirs(t *testing.T) (vault, data string) {
	t.Helper()
	base := t.TempDir()
	vault = filepath.Join(base, "vault")
	data = filepath.Join(base, "data")
	if err := os.Mkdir(vault, 0o755); err != nil {
		t.Fatal(err)
	}
	return vault, data
}

func TestDefaults(t *testing.T) {
	vault, data := dirs(t)
	c, err := Load(env(map[string]string{"VAULT_DIR": vault, "DATA_DIR": data}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != 8080 || c.LogLevel != slog.LevelInfo || c.LogFormat != "json" || c.ShutdownTimeout != 20*time.Second {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.MaxNoteBytes != 5<<20 || c.Auth.Mode != "password" || c.Auth.SessionTTL != 30*24*time.Hour ||
		c.Auth.CookieSecure != "auto" || c.Auth.ProxyHeader != "X-Forwarded-User" {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if st, err := os.Stat(data); err != nil || !st.IsDir() {
		t.Errorf("DATA_DIR was not created: %v", err)
	}
}

func TestValuesParsed(t *testing.T) {
	vault, data := dirs(t)
	c, err := Load(env(map[string]string{
		"VAULT_DIR": vault, "DATA_DIR": data,
		"PORT": "9000", "LOG_LEVEL": "debug", "LOG_FORMAT": "TEXT", "SHUTDOWN_TIMEOUT": "5s",
		"TRUSTED_PROXIES": "10.0.0.0/8, 192.168.1.10", "MAX_NOTE_MB": "12",
		"IGNORE": "node_modules, *.tmp", "SESSION_TTL": "12h", "COOKIE_SECURE": "true",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != 9000 || c.LogLevel != slog.LevelDebug || c.LogFormat != "text" || c.ShutdownTimeout != 5*time.Second {
		t.Errorf("values not applied: %+v", c)
	}
	if len(c.TrustedProxies) != 2 || c.TrustedProxies[1].String() != "192.168.1.10/32" {
		t.Errorf("TRUSTED_PROXIES: %v", c.TrustedProxies)
	}
	if c.MaxNoteBytes != 12<<20 || len(c.Ignore) != 2 || c.Ignore[1] != "*.tmp" {
		t.Errorf("MAX_NOTE_MB/IGNORE: %d %v", c.MaxNoteBytes, c.Ignore)
	}
	if c.Auth.SessionTTL != 12*time.Hour || c.Auth.CookieSecure != "true" {
		t.Errorf("auth values: %+v", c.Auth)
	}
}

func TestInvalidValues(t *testing.T) {
	vault, data := dirs(t)
	cases := map[string]string{
		"PORT":             "0",
		"LOG_LEVEL":        "loud",
		"LOG_FORMAT":       "xml",
		"SHUTDOWN_TIMEOUT": "-1s",
		"TRUSTED_PROXIES":  "10.0.0.0/99",
		"MAX_NOTE_MB":      "0",
		"IGNORE":           "a/b",
		"SESSION_TTL":      "1m",
		"COOKIE_SECURE":    "maybe",
		"AUTH_MODE":        "ldap",
	}
	for key, val := range cases {
		t.Run(key, func(t *testing.T) {
			_, err := Load(env(map[string]string{"VAULT_DIR": vault, "DATA_DIR": data, key: val}))
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("expected error about %s, got: %v", key, err)
			}
		})
	}
}

func TestPasswordSources(t *testing.T) {
	vault, data := dirs(t)
	dir := t.TempDir()
	pwFile := filepath.Join(dir, "password")
	hashFile := filepath.Join(dir, "hash")
	shortFile := filepath.Join(dir, "short")
	os.WriteFile(pwFile, []byte("long enough password\n"), 0o600)
	os.WriteFile(hashFile, []byte("$argon2id$from-file\n"), 0o600)
	os.WriteFile(shortFile, []byte("short\n"), 0o600)

	base := func(extra map[string]string) map[string]string {
		m := map[string]string{"VAULT_DIR": vault, "DATA_DIR": data, "AUTH_MODE": "password"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	c, err := Load(env(base(map[string]string{"AUTH_PASSWORD_FILE": pwFile})))
	if err != nil || c.Auth.Password.Reveal() != "long enough password" {
		t.Errorf("password file: %q %v", c.Auth.Password.Reveal(), err)
	}
	c, err = Load(env(base(map[string]string{"AUTH_PASSWORD_HASH_FILE": hashFile})))
	if err != nil || c.Auth.PasswordHash != "$argon2id$from-file" {
		t.Errorf("hash file: %q %v", c.Auth.PasswordHash, err)
	}
	if _, err := Load(env(base(nil))); err == nil || !strings.Contains(err.Error(), "hash-password") {
		t.Errorf("missing password: %v", err)
	}
	if _, err := Load(env(base(map[string]string{"AUTH_PASSWORD_FILE": pwFile, "AUTH_PASSWORD_HASH": "x"}))); err == nil {
		t.Error("two password sources accepted")
	}
	if _, err := Load(env(base(map[string]string{"AUTH_PASSWORD_FILE": shortFile}))); err == nil {
		t.Error("short password accepted")
	}
	if _, err := Load(env(base(map[string]string{"AUTH_PASSWORD_FILE": filepath.Join(dir, "missing")}))); err == nil {
		t.Error("missing password file accepted")
	}
}

func TestProxyMode(t *testing.T) {
	vault, data := dirs(t)
	m := map[string]string{"VAULT_DIR": vault, "DATA_DIR": data, "AUTH_MODE": "proxy"}
	if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
		t.Errorf("proxy mode without trusted proxies: %v", err)
	}
	m["TRUSTED_PROXIES"] = "10.0.0.0/8"
	m["AUTH_PROXY_ALLOWED_USERS"] = "erik, anna"
	m["AUTH_PROXY_LOGOUT_URL"] = "/oauth2/sign_out"
	c, err := Load(env(m))
	if err != nil || len(c.Auth.ProxyAllowed) != 2 || c.Auth.ProxyLogoutURL != "/oauth2/sign_out" {
		t.Errorf("proxy mode: %+v %v", c.Auth, err)
	}
	for _, bad := range []string{"javascript:alert(1)", "//evil.example", "data:text/html,x", "/a b"} {
		m["AUTH_PROXY_LOGOUT_URL"] = bad
		if _, err := Load(env(m)); err == nil {
			t.Errorf("logout URL %q accepted", bad)
		}
	}
}

func TestSecretIsRedacted(t *testing.T) {
	s := Secret("hunter2hunter2")
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "pw", s, "auth", Auth{Password: s})
	out := buf.String() + fmt.Sprintf("%v %+v %#v %s", s, Auth{Password: s}, s, s)
	if strings.Contains(out, "hunter2") {
		t.Fatalf("secret leaked: %s", out)
	}
}

func TestVaultMissing(t *testing.T) {
	base := t.TempDir()
	_, err := Load(env(map[string]string{"VAULT_DIR": filepath.Join(base, "missing"), "DATA_DIR": filepath.Join(base, "data")}))
	if err == nil || !strings.Contains(err.Error(), "VAULT_DIR") {
		t.Fatalf("expected VAULT_DIR error, got: %v", err)
	}
}

func TestVaultIsFile(t *testing.T) {
	base := t.TempDir()
	f := filepath.Join(base, "file")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(env(map[string]string{"VAULT_DIR": f, "DATA_DIR": filepath.Join(base, "data")})); err == nil {
		t.Fatal("expected error for a file as vault")
	}
}

func TestDataInsideVaultRejected(t *testing.T) {
	vault, _ := dirs(t)
	for _, data := range []string{vault, filepath.Join(vault, ".leafnotes")} {
		_, err := Load(env(map[string]string{"VAULT_DIR": vault, "DATA_DIR": data}))
		if err == nil || !strings.Contains(err.Error(), "must not be inside") {
			t.Errorf("DATA_DIR=%s: expected rejection, got: %v", data, err)
		}
	}
}

func TestDataInsideVaultViaSymlinkRejected(t *testing.T) {
	vault, _ := dirs(t)
	inside := filepath.Join(vault, "cache")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "data-link")
	if err := os.Symlink(inside, link); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if _, err := Load(env(map[string]string{"VAULT_DIR": vault, "DATA_DIR": link})); err == nil {
		t.Error("symlink into the vault as DATA_DIR was not detected")
	}
}

func TestSiblingWithSamePrefixAllowed(t *testing.T) {
	base := t.TempDir()
	vault := filepath.Join(base, "vault")
	data := filepath.Join(base, "vault-data") // same prefix, but not a subdirectory
	if err := os.Mkdir(vault, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(env(map[string]string{"VAULT_DIR": vault, "DATA_DIR": data})); err != nil {
		t.Errorf("sibling directory wrongly rejected: %v", err)
	}
}

func TestDataDirNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions are not enforced for root")
	}
	vault, data := dirs(t)
	if err := os.Mkdir(data, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(data, 0o700) })
	_, err := Load(env(map[string]string{"VAULT_DIR": vault, "DATA_DIR": data}))
	if err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("expected a permissions error, got: %v", err)
	}
}
