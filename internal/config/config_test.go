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

// env builds a getenv function; a password hash is set unless the test
// configures authentication itself, because every valid configuration needs it.
func env(m map[string]string) func(string) string {
	authSet := false
	for _, k := range []string{EnvAuthMode, EnvPasswordFile, EnvPasswordHashFile, EnvPasswordHash} {
		if _, ok := m[k]; ok {
			authSet = true
		}
	}
	if !authSet {
		m[EnvPasswordHash] = "$argon2id$placeholder"
	}
	return func(k string) string { return m[k] }
}

func dirs(t *testing.T) (soma, data string) {
	t.Helper()
	base := t.TempDir()
	soma = filepath.Join(base, "soma")
	data = filepath.Join(base, "data")
	if err := os.Mkdir(soma, 0o755); err != nil {
		t.Fatal(err)
	}
	return soma, data
}

func TestDefaults(t *testing.T) {
	soma, data := dirs(t)
	c, err := Load(env(map[string]string{EnvSomaDir: soma, EnvDataDir: data}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != 8080 || c.LogLevel != slog.LevelInfo || c.LogFormat != "json" || c.ShutdownTimeout != 20*time.Second ||
		c.RescanInterval != 5*time.Minute {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.MaxNoteBytes != 5<<20 || c.MaxUploadBytes != 25<<20 || c.Auth.Mode != "password" || c.Auth.SessionTTL != 30*24*time.Hour ||
		c.Auth.CookieSecure != "auto" || c.Auth.ProxyHeader != "X-Forwarded-User" {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if DefaultSomaDir != "/soma" || DefaultDataDir != "/data" {
		t.Errorf("default directories: %s %s", DefaultSomaDir, DefaultDataDir)
	}
	if st, err := os.Stat(data); err != nil || !st.IsDir() {
		t.Errorf("data directory was not created: %v", err)
	}
}

func TestVariableNames(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range Vars {
		if !strings.HasPrefix(v, "DENDRONOTES_") || strings.ToUpper(v) != v {
			t.Errorf("%s: variables must be uppercase with the DENDRONOTES_ prefix", v)
		}
		if seen[v] {
			t.Errorf("%s listed twice", v)
		}
		seen[v] = true
	}
	// Kubernetes injects DENDRONOTES_PORT and DENDRONOTES_SERVICE_* for a
	// Service named dendronotes; none of our names may collide with them.
	for _, v := range Vars {
		if v == "DENDRONOTES_PORT" || strings.HasPrefix(v, "DENDRONOTES_PORT_") || strings.HasPrefix(v, "DENDRONOTES_SERVICE_") {
			t.Errorf("%s collides with Kubernetes service environment variables", v)
		}
	}
	if EnvSomaDir != "DENDRONOTES_SOMA_DIR" || EnvHTTPPort != "DENDRONOTES_HTTP_PORT" {
		t.Errorf("unexpected names: %s %s", EnvSomaDir, EnvHTTPPort)
	}
}

func TestOnlyPrefixedVariablesAreRead(t *testing.T) {
	soma, data := dirs(t)
	c, err := Load(env(map[string]string{
		EnvSomaDir: soma, EnvDataDir: data,
		// Unprefixed look-alikes and the Kubernetes service variable are ignored.
		"SOMA_DIR": "/elsewhere", "HTTP_PORT": "1", "PORT": "2", "LOG_LEVEL": "error",
		"DENDRONOTES_PORT": "tcp://10.0.0.1:8080",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SomaDir != soma || c.Port != 8080 || c.LogLevel != slog.LevelInfo {
		t.Errorf("unprefixed variables were used: %+v", c)
	}
}

func TestReadmeDocumentsAllVariables(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range Vars {
		if !bytes.Contains(readme, []byte("`"+v+"`")) {
			t.Errorf("README.md does not document %s", v)
		}
	}
}

func TestValuesParsed(t *testing.T) {
	soma, data := dirs(t)
	c, err := Load(env(map[string]string{
		EnvSomaDir: soma, EnvDataDir: data,
		EnvHTTPPort: "9000", EnvLogLevel: "debug", EnvLogFormat: "TEXT", EnvShutdownTimeout: "5s",
		EnvTrustedProxies: "10.0.0.0/8, 192.168.1.10", EnvMaxNoteMB: "12",
		EnvIgnore: "node_modules, *.tmp", EnvSessionTTL: "12h", EnvCookieSecure: "true",
		EnvRescanInterval: "0", EnvMaxUploadMB: "100",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != 9000 || c.LogLevel != slog.LevelDebug || c.LogFormat != "text" || c.ShutdownTimeout != 5*time.Second ||
		c.RescanInterval != 0 {
		t.Errorf("values not applied: %+v", c)
	}
	if len(c.TrustedProxies) != 2 || c.TrustedProxies[1].String() != "192.168.1.10/32" {
		t.Errorf("trusted proxies: %v", c.TrustedProxies)
	}
	if c.MaxNoteBytes != 12<<20 || c.MaxUploadBytes != 100<<20 || len(c.Ignore) != 2 || c.Ignore[1] != "*.tmp" {
		t.Errorf("max note size / ignore: %d %v", c.MaxNoteBytes, c.Ignore)
	}
	if c.Auth.SessionTTL != 12*time.Hour || c.Auth.CookieSecure != "true" {
		t.Errorf("auth values: %+v", c.Auth)
	}
}

func TestInvalidValues(t *testing.T) {
	soma, data := dirs(t)
	cases := map[string]string{
		EnvHTTPPort:        "0",
		EnvLogLevel:        "loud",
		EnvLogFormat:       "xml",
		EnvShutdownTimeout: "-1s",
		EnvTrustedProxies:  "10.0.0.0/99",
		EnvMaxNoteMB:       "0",
		EnvIgnore:          "a/b",
		EnvRescanInterval:  "1s",
		EnvMaxUploadMB:     "0",
		EnvSessionTTL:      "1m",
		EnvCookieSecure:    "maybe",
		EnvAuthMode:        "ldap",
	}
	for key, val := range cases {
		t.Run(key, func(t *testing.T) {
			_, err := Load(env(map[string]string{EnvSomaDir: soma, EnvDataDir: data, key: val}))
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("expected an error naming %s, got: %v", key, err)
			}
		})
	}
}

func TestPasswordSources(t *testing.T) {
	soma, data := dirs(t)
	dir := t.TempDir()
	pwFile := filepath.Join(dir, "password")
	hashFile := filepath.Join(dir, "hash")
	shortFile := filepath.Join(dir, "short")
	os.WriteFile(pwFile, []byte("long enough password\n"), 0o600)
	os.WriteFile(hashFile, []byte("$argon2id$from-file\n"), 0o600)
	os.WriteFile(shortFile, []byte("short\n"), 0o600)

	base := func(extra map[string]string) map[string]string {
		m := map[string]string{EnvSomaDir: soma, EnvDataDir: data, EnvAuthMode: "password"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	c, err := Load(env(base(map[string]string{EnvPasswordFile: pwFile})))
	if err != nil || c.Auth.Password.Reveal() != "long enough password" {
		t.Errorf("password file: %q %v", c.Auth.Password.Reveal(), err)
	}
	c, err = Load(env(base(map[string]string{EnvPasswordHashFile: hashFile})))
	if err != nil || c.Auth.PasswordHash != "$argon2id$from-file" {
		t.Errorf("hash file: %q %v", c.Auth.PasswordHash, err)
	}
	if _, err := Load(env(base(nil))); err == nil || !strings.Contains(err.Error(), "dendronotes hash-password") {
		t.Errorf("missing password: %v", err)
	}
	if _, err := Load(env(base(map[string]string{EnvPasswordFile: pwFile, EnvPasswordHash: "x"}))); err == nil {
		t.Error("two password sources accepted")
	}
	if _, err := Load(env(base(map[string]string{EnvPasswordFile: shortFile}))); err == nil {
		t.Error("short password accepted")
	}
	if _, err := Load(env(base(map[string]string{EnvPasswordFile: filepath.Join(dir, "missing")}))); err == nil {
		t.Error("missing password file accepted")
	}
}

func TestProxyMode(t *testing.T) {
	soma, data := dirs(t)
	m := map[string]string{EnvSomaDir: soma, EnvDataDir: data, EnvAuthMode: "proxy"}
	if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), EnvTrustedProxies) {
		t.Errorf("proxy mode without trusted proxies: %v", err)
	}
	m[EnvTrustedProxies] = "10.0.0.0/8"
	m[EnvProxyAllowed] = "erik, anna"
	m[EnvProxyLogoutURL] = "/oauth2/sign_out"
	c, err := Load(env(m))
	if err != nil || len(c.Auth.ProxyAllowed) != 2 || c.Auth.ProxyLogoutURL != "/oauth2/sign_out" {
		t.Errorf("proxy mode: %+v %v", c.Auth, err)
	}
	for _, bad := range []string{"javascript:alert(1)", "//evil.example", "data:text/html,x", "/a b"} {
		m[EnvProxyLogoutURL] = bad
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

func TestSomaMissing(t *testing.T) {
	base := t.TempDir()
	_, err := Load(env(map[string]string{EnvSomaDir: filepath.Join(base, "missing"), EnvDataDir: filepath.Join(base, "data")}))
	if err == nil || !strings.Contains(err.Error(), EnvSomaDir) {
		t.Fatalf("expected an error naming %s, got: %v", EnvSomaDir, err)
	}
}

func TestSomaIsFile(t *testing.T) {
	base := t.TempDir()
	f := filepath.Join(base, "file")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(env(map[string]string{EnvSomaDir: f, EnvDataDir: filepath.Join(base, "data")})); err == nil {
		t.Fatal("expected an error for a file as soma")
	}
}

func TestDataInsideSomaRejected(t *testing.T) {
	soma, _ := dirs(t)
	for _, data := range []string{soma, filepath.Join(soma, ".dendronotes")} {
		_, err := Load(env(map[string]string{EnvSomaDir: soma, EnvDataDir: data}))
		if err == nil || !strings.Contains(err.Error(), "must not be inside") {
			t.Errorf("data dir %s: expected rejection, got: %v", data, err)
		}
	}
}

func TestDataInsideSomaViaSymlinkRejected(t *testing.T) {
	soma, _ := dirs(t)
	inside := filepath.Join(soma, "cache")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "data-link")
	if err := os.Symlink(inside, link); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if _, err := Load(env(map[string]string{EnvSomaDir: soma, EnvDataDir: link})); err == nil {
		t.Error("a symlink into the soma as data dir was not detected")
	}
}

func TestSiblingWithSamePrefixAllowed(t *testing.T) {
	base := t.TempDir()
	soma := filepath.Join(base, "soma")
	data := filepath.Join(base, "soma-data") // same prefix, but not a subdirectory
	if err := os.Mkdir(soma, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(env(map[string]string{EnvSomaDir: soma, EnvDataDir: data})); err != nil {
		t.Errorf("sibling directory wrongly rejected: %v", err)
	}
}

func TestDataDirNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions are not enforced for root")
	}
	soma, data := dirs(t)
	if err := os.Mkdir(data, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(data, 0o700) })
	_, err := Load(env(map[string]string{EnvSomaDir: soma, EnvDataDir: data}))
	if err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("expected a permissions error, got: %v", err)
	}
}
