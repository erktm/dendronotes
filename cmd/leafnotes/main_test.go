package main

import (
	"bytes"
	"strings"
	"testing"

	"leafnotes/internal/auth"
)

func TestCommands(t *testing.T) {
	noEnv := func(string) string { return "" }

	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, noEnv, nil, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != version {
		t.Errorf("version: code %d, output %q", code, out.String())
	}

	out.Reset()
	if code := run([]string{"bogus"}, noEnv, nil, &out, &errOut); code != 2 {
		t.Errorf("unknown command: code %d", code)
	}

	// serve without a valid configuration must end with an error code and a
	// structured log line, not with a panic.
	out.Reset()
	env := func(k string) string {
		switch k {
		case "VAULT_DIR":
			return t.TempDir() + "/missing"
		case "DATA_DIR":
			return t.TempDir()
		}
		return ""
	}
	if code := run([]string{"serve"}, env, nil, &out, &errOut); code != 1 {
		t.Errorf("serve with missing vault: code %d", code)
	}
	if !strings.Contains(out.String(), `"msg":"invalid configuration"`) {
		t.Errorf("no JSON log about the configuration: %q", out.String())
	}
}

func TestHashPassword(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"hash-password"}, nil, strings.NewReader("a long passphrase\n"), &out, &errOut)
	if code != 0 {
		t.Fatalf("code %d: %s", code, errOut.String())
	}
	h, err := auth.ParseHash(strings.TrimSpace(out.String()))
	if err != nil {
		t.Fatalf("output is not a valid hash: %v", err)
	}
	if !h.Verify("a long passphrase") {
		t.Fatal("hash does not verify the password")
	}

	out.Reset()
	if code := run([]string{"hash-password"}, nil, strings.NewReader("short\n"), &out, &errOut); code != 1 || out.Len() != 0 {
		t.Errorf("short password: code %d, output %q", code, out.String())
	}
}
