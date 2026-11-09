package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/auth"
	"dendronotes/internal/config"
	"dendronotes/internal/history"
	"dendronotes/internal/store"
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
		case config.EnvSomaDir:
			return t.TempDir() + "/missing"
		case config.EnvDataDir:
			return t.TempDir()
		}
		return ""
	}
	if code := run([]string{"serve"}, env, nil, &out, &errOut); code != 1 {
		t.Errorf("serve with missing soma: code %d", code)
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

func TestResetPasswordAndBackup(t *testing.T) {
	dir := t.TempDir()
	env := func(k string) string {
		switch k {
		case config.EnvDataDir:
			return dir
		case config.EnvAdminUsername:
			return "boss"
		}
		return ""
	}
	ctx := context.Background()
	st, err := store.Open(dir, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	old, _ := auth.HashPassword("the old password", auth.DefaultParams)
	boss, err := st.CreateUser(ctx, store.User{Username: "boss", Role: store.RoleAdmin, Source: store.SourceLocal, PasswordHash: old, Disabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, store.User{Username: "sso", Role: store.RoleUser, Source: store.SourceOIDC, OIDCIssuer: "i", OIDCSubject: "s"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(ctx, boss.ID, time.Hour, "", ""); err != nil {
		t.Fatal(err)
	}
	st.Close()
	os.Mkdir(filepath.Join(dir, "history"), 0o750)
	h, err := history.Open(history.FileName(dir, "somaid0001"))
	if err == nil {
		err = h.Record(ctx, "Plan.md", []byte("beds"), nil, history.ReasonCreate)
		h.Close()
	}
	if err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	for _, c := range []struct {
		args  []string
		stdin string
	}{
		{[]string{"reset-password", "nobody"}, "a long new password\n"},
		{[]string{"reset-password", "sso"}, "a long new password\n"},
		{[]string{"reset-password"}, "too short\n"},
		{[]string{"reset-password", "a", "b"}, ""},
	} {
		if code := run(c.args, env, strings.NewReader(c.stdin), &out, &errOut); code != 1 {
			t.Errorf("%v: code %d", c.args, code)
		}
	}
	errOut.Reset()
	if code := run([]string{"reset-password"}, env, strings.NewReader("a long new password\n"), &out, &errOut); code != 0 {
		t.Fatalf("reset: code %d: %s", code, errOut.String())
	}
	st, err = store.Open(dir, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := st.UserByID(ctx, boss.ID)
	if h, err := auth.ParseHash(u.PasswordHash); err != nil || !h.Verify("a long new password") || u.Disabled {
		t.Errorf("after reset: %+v %v", u, err)
	}
	if sessions, _ := st.Sessions(ctx); len(sessions) != 0 {
		t.Errorf("sessions survived: %d", len(sessions))
	}
	st.Close()

	// The backup holds both databases and opens as they were.
	file := filepath.Join(t.TempDir(), "b.zip")
	if code := run([]string{"backup", file}, env, nil, &out, &errOut); code != 0 {
		t.Fatalf("backup: code %d: %s", code, errOut.String())
	}
	if code := run([]string{"backup", file}, env, nil, &out, &errOut); code != 1 {
		t.Errorf("backup over an existing file: code %d", code)
	}
	zr, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	restored := t.TempDir()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		if f.Name == "manifest.json" {
			continue
		}
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		os.MkdirAll(filepath.Dir(filepath.Join(restored, f.Name)), 0o755)
		os.WriteFile(filepath.Join(restored, f.Name), data, 0o600)
	}
	if strings.Join(names, ",") != "state.db,history/somaid0001.db,manifest.json" {
		t.Errorf("backup contents: %v", names)
	}
	st, err = store.Open(restored, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.UserByUsername(ctx, "boss"); err != nil {
		t.Errorf("restored state: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) == 0 {
		t.Error("data dir emptied")
	} else {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".backup-") {
				t.Errorf("temporary copy left: %s", e.Name())
			}
		}
	}
}
