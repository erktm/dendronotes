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

// env builds a getenv function from a map.
func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func dirs(t *testing.T) (soma, data string) {
	t.Helper()
	base := t.TempDir()
	soma = filepath.Join(base, "somas")
	data = filepath.Join(base, "data")
	if err := os.Mkdir(soma, 0o755); err != nil {
		t.Fatal(err)
	}
	return soma, data
}

func TestDefaults(t *testing.T) {
	soma, data := dirs(t)
	c, err := Load(env(map[string]string{EnvSomasDir: soma, EnvDataDir: data}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != 8080 || c.LogLevel != slog.LevelInfo || c.LogFormat != "json" || c.ShutdownTimeout != 20*time.Second ||
		c.RescanInterval != 5*time.Minute {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.MaxNoteBytes != 5<<20 || c.MaxUploadBytes != 25<<20 || c.Auth.Mode != "local" || c.Auth.AdminUsername != "admin" || c.Auth.SessionTTL != 30*24*time.Hour ||
		c.Auth.CookieSecure != "auto" || c.Auth.ProxyHeader != "X-Forwarded-User" {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if DefaultSomasDir != "/somas" || DefaultDataDir != "/data" {
		t.Errorf("default directories: %s %s", DefaultSomasDir, DefaultDataDir)
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
		EnvSomasDir: soma, EnvDataDir: data,
		// Unprefixed look-alikes and the Kubernetes service variable are ignored.
		"SOMA_DIR": "/elsewhere", "HTTP_PORT": "1", "PORT": "2", "LOG_LEVEL": "error",
		"DENDRONOTES_PORT": "tcp://10.0.0.1:8080",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SomasDir != soma || c.ImportSomaDir != "" || c.Port != 8080 || c.LogLevel != slog.LevelInfo {
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
		EnvSomasDir: soma, EnvDataDir: data,
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
			_, err := Load(env(map[string]string{EnvSomasDir: soma, EnvDataDir: data, key: val}))
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("expected an error naming %s, got: %v", key, err)
			}
		})
	}
}

func TestAdminPasswordSources(t *testing.T) {
	soma, data := dirs(t)
	dir := t.TempDir()
	pwFile := filepath.Join(dir, "password")
	hashFile := filepath.Join(dir, "hash")
	shortFile := filepath.Join(dir, "short")
	os.WriteFile(pwFile, []byte("long enough password\n"), 0o600)
	os.WriteFile(hashFile, []byte("$argon2id$from-file\n"), 0o600)
	os.WriteFile(shortFile, []byte("short\n"), 0o600)

	base := func(extra map[string]string) map[string]string {
		m := map[string]string{EnvSomasDir: soma, EnvDataDir: data}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	c, err := Load(env(base(map[string]string{EnvAdminPassword: pwFile, EnvAdminUsername: "erik"})))
	if err != nil || c.Auth.AdminPassword.Reveal() != "long enough password" || c.Auth.AdminUsername != "erik" {
		t.Errorf("password file: %q %v", c.Auth.AdminPassword.Reveal(), err)
	}
	c, err = Load(env(base(map[string]string{EnvAdminHashFile: hashFile})))
	if err != nil || c.Auth.AdminHash != "$argon2id$from-file" {
		t.Errorf("hash file: %q %v", c.Auth.AdminHash, err)
	}
	// Optional: only needed while no admin exists (the server checks that).
	if _, err := Load(env(base(nil))); err != nil {
		t.Errorf("no admin password: %v", err)
	}
	if _, err := Load(env(base(map[string]string{EnvAdminPassword: pwFile, EnvAdminHash: "x"}))); err == nil {
		t.Error("two password sources accepted")
	}
	if _, err := Load(env(base(map[string]string{EnvAdminPassword: shortFile}))); err == nil {
		t.Error("short password accepted")
	}
	if _, err := Load(env(base(map[string]string{EnvAdminPassword: filepath.Join(dir, "missing")}))); err == nil {
		t.Error("missing password file accepted")
	}
}

func TestProxyMode(t *testing.T) {
	soma, data := dirs(t)
	m := map[string]string{EnvSomasDir: soma, EnvDataDir: data, EnvAuthMode: "proxy"}
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
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "pw", s, "auth", Auth{AdminPassword: s})
	out := buf.String() + fmt.Sprintf("%v %+v %#v %s", s, Auth{AdminPassword: s}, s, s)
	if strings.Contains(out, "hunter2") {
		t.Fatalf("secret leaked: %s", out)
	}
}

func TestSomasDirCreatedAndImportChecked(t *testing.T) {
	base := t.TempDir()
	somas := filepath.Join(base, "somas")
	data := filepath.Join(base, "data")
	c, err := Load(env(map[string]string{EnvSomasDir: somas, EnvDataDir: data}))
	if err != nil || c.SomasDir != somas {
		t.Fatalf("Load: %v", err)
	}
	if st, err := os.Stat(somas); err != nil || !st.IsDir() {
		t.Errorf("somas directory was not created: %v", err)
	}
	_, err = Load(env(map[string]string{EnvSomasDir: somas, EnvDataDir: data, EnvSomaDir: filepath.Join(base, "missing")}))
	if err == nil || !strings.Contains(err.Error(), EnvSomaDir) {
		t.Fatalf("expected an error naming %s, got: %v", EnvSomaDir, err)
	}
	legacy := filepath.Join(base, "old-soma")
	os.Mkdir(legacy, 0o755)
	if c, err := Load(env(map[string]string{EnvSomasDir: somas, EnvDataDir: data, EnvSomaDir: legacy})); err != nil || c.ImportSomaDir != legacy {
		t.Errorf("import soma: %q %v", c.ImportSomaDir, err)
	}
	if _, err := Load(env(map[string]string{EnvSomasDir: somas, EnvDataDir: filepath.Join(legacy, "data"), EnvSomaDir: legacy})); err == nil {
		t.Error("data inside the imported soma accepted")
	}
}

func TestSomaIsFile(t *testing.T) {
	base := t.TempDir()
	f := filepath.Join(base, "file")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(env(map[string]string{EnvSomasDir: f, EnvDataDir: filepath.Join(base, "data")})); err == nil {
		t.Fatal("expected an error for a file as soma")
	}
}

func TestDataInsideSomaRejected(t *testing.T) {
	soma, _ := dirs(t)
	for _, data := range []string{soma, filepath.Join(soma, ".dendronotes")} {
		_, err := Load(env(map[string]string{EnvSomasDir: soma, EnvDataDir: data}))
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
	if _, err := Load(env(map[string]string{EnvSomasDir: soma, EnvDataDir: link})); err == nil {
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
	if _, err := Load(env(map[string]string{EnvSomasDir: soma, EnvDataDir: data})); err != nil {
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
	_, err := Load(env(map[string]string{EnvSomasDir: soma, EnvDataDir: data}))
	if err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("expected a permissions error, got: %v", err)
	}
}

func TestDescribeHidesSecrets(t *testing.T) {
	soma, data := dirs(t)
	pw := filepath.Join(t.TempDir(), "pw")
	os.WriteFile(pw, []byte("very secret password"), 0o600)
	c, err := Load(env(map[string]string{EnvSomasDir: soma, EnvDataDir: data, EnvAdminPassword: pw, EnvMaxUploadMB: "40"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range Vars {
		if strings.Contains(c.Describe(v), "secret") {
			t.Errorf("%s reveals the secret", v)
		}
	}
	if c.Describe(EnvAdminPassword) != "(set)" || c.Describe(EnvMaxUploadMB) != "40" || c.Describe(EnvSomasDir) != soma {
		t.Errorf("descriptions: %q %q %q", c.Describe(EnvAdminPassword), c.Describe(EnvMaxUploadMB), c.Describe(EnvSomasDir))
	}
	for _, v := range Vars {
		if v != EnvSomaDir && v != EnvTrustedProxies && v != EnvIgnore && v != EnvProxyAllowed && v != EnvProxyLogoutURL &&
			v != EnvAdminHash && v != EnvAdminHashFile && v != EnvPublicURL && !strings.HasPrefix(v, Prefix+"OIDC_") &&
			c.Describe(v) == "" {
			t.Errorf("%s has no description", v)
		}
	}
}

func TestOIDC(t *testing.T) {
	soma, data := dirs(t)
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := map[string]string{EnvSomasDir: soma, EnvDataDir: data, EnvOIDCIssuer: "https://sso.example.org/realms/main/"}
	_, err := Load(env(m))
	if err == nil || !strings.Contains(err.Error(), EnvOIDCClientID) || !strings.Contains(err.Error(), EnvPublicURL) {
		t.Errorf("issuer alone: %v", err)
	}
	m[EnvOIDCClientID] = "dendronotes"
	m[EnvPublicURL] = "https://notes.example.org/"
	m[EnvOIDCSecretFile] = secret
	m[EnvOIDCScopes] = "profile,email groups"
	m[EnvOIDCAdminGroups] = "dn-admins"
	c, err := Load(env(m))
	o := c.Auth.OIDC
	if err != nil || !o.Enabled() || o.Issuer != "https://sso.example.org/realms/main" || c.Auth.PublicURL != "https://notes.example.org" ||
		o.ClientSecret.Reveal() != "s3cret" || strings.Join(o.Scopes, " ") != "openid profile email groups" ||
		o.UsernameClaim != "preferred_username" || o.GroupsClaim != "groups" || o.LoginLabel != "SSO" ||
		len(o.AdminGroups) != 1 || !c.Auth.LocalLogin {
		t.Errorf("oidc: %+v %v", c.Auth, err)
	}
	if c.Describe(EnvOIDCSecretFile) != "(set)" || strings.Contains(c.Describe(EnvOIDCSecretFile), "s3cret") {
		t.Errorf("secret shown: %q", c.Describe(EnvOIDCSecretFile))
	}

	m[EnvLocalLogin] = "false"
	if c, err := Load(env(m)); err != nil || c.Auth.LocalLogin {
		t.Errorf("local login off: %v", err)
	}
	m[EnvAuthMode] = "proxy"
	m[EnvTrustedProxies] = "10.0.0.0/8"
	if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), "only with") {
		t.Errorf("oidc in proxy mode: %v", err)
	}

	for k, bad := range map[string]string{
		EnvPublicURL:  "notes.example.org",
		EnvOIDCIssuer: "ftp://sso.example.org",
		EnvLocalLogin: "maybe",
		EnvOIDCCAFile: secret, // not PEM
	} {
		m := map[string]string{EnvSomasDir: soma, EnvDataDir: data, EnvOIDCIssuer: "https://sso.example.org", EnvOIDCClientID: "x",
			EnvPublicURL: "https://notes.example.org", k: bad}
		if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), k) {
			t.Errorf("%s=%q: %v", k, bad, err)
		}
	}
	// Turning passwords off needs another way in.
	if _, err := Load(env(map[string]string{EnvSomasDir: soma, EnvDataDir: data, EnvLocalLogin: "false"})); err == nil {
		t.Error("LOCAL_LOGIN=false without OIDC accepted")
	}
}
