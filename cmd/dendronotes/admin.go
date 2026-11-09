package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"dendronotes/internal/auth"
	"dendronotes/internal/backup"
	"dendronotes/internal/config"
	"dendronotes/internal/store"
)

// Maintenance commands that work on DATA_DIR directly, also while the server
// runs (kubectl exec). They read only DATA_DIR and ADMIN_USERNAME, so a
// broken setting elsewhere cannot lock the admin out.

func dataDir(getenv func(string) string) string {
	if v := getenv(config.EnvDataDir); v != "" {
		return v
	}
	return config.DefaultDataDir
}

func openState(getenv func(string) string) (*store.Store, error) {
	dir := dataDir(getenv)
	if _, err := os.Stat(dir + "/" + store.FileName); err != nil {
		return nil, fmt.Errorf("no %s in %s (set %s)", store.FileName, dir, config.EnvDataDir)
	}
	return store.Open(dir, slog.New(slog.DiscardHandler))
}

// resetPassword sets a new password for a local account (by default the
// admin account), enables it and ends its sessions.
func resetPassword(args []string, getenv func(string) string, stdin io.Reader, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	name := getenv(config.EnvAdminUsername)
	if name == "" {
		name = config.DefaultAdminUsername
	}
	switch len(args) {
	case 0:
	case 1:
		name = args[0]
	default:
		return fail(errors.New("usage: dendronotes reset-password [user name]"))
	}
	st, err := openState(getenv)
	if err != nil {
		return fail(err)
	}
	defer st.Close()
	ctx := context.Background()
	u, err := st.UserByUsername(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return fail(fmt.Errorf("there is no account %q", name))
	}
	if err != nil {
		return fail(err)
	}
	if u.Source != store.SourceLocal {
		return fail(fmt.Errorf("%s signs in through %s and has no password in dendronotes", u.Username, u.Source))
	}
	fmt.Fprintf(stderr, "New password for %s (at least %d characters).\n", u.Username, auth.MinPasswordLength)
	pw, err := readPassword(stdin, stderr)
	if err != nil {
		return fail(err)
	}
	if len(pw) < auth.MinPasswordLength {
		return fail(fmt.Errorf("use at least %d characters", auth.MinPasswordLength))
	}
	hash, err := auth.HashPassword(pw, auth.DefaultParams)
	if err != nil {
		return fail(err)
	}
	enabled := false
	if _, err := st.UpdateUser(ctx, u.ID, store.UserUpdate{PasswordHash: &hash, Disabled: &enabled}); err != nil {
		return fail(err)
	}
	if err := st.DeleteUserSessions(ctx, u.ID, 0); err != nil {
		return fail(err)
	}
	st.Audit(ctx, store.AuditEntry{Actor: "command line", Action: "user.password.reset", Target: u.Username})
	fmt.Fprintf(stdout, "The password of %s was set; the account is enabled and signed out everywhere.\n", u.Username)
	return 0
}

// backupState writes a backup ZIP of state.db and the histories to a new file.
func backupState(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if len(args) != 1 {
		return fail(errors.New("usage: dendronotes backup <file.zip>"))
	}
	st, err := openState(getenv)
	if err != nil {
		return fail(err)
	}
	defer st.Close()
	snap, err := backup.Take(context.Background(), st, dataDir(getenv))
	if err != nil {
		return fail(err)
	}
	defer snap.Close()
	f, err := os.OpenFile(args[0], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fail(err)
	}
	if err := snap.WriteZip(f, version); err != nil {
		f.Close()
		os.Remove(args[0])
		return fail(err)
	}
	if err := f.Close(); err != nil {
		return fail(err)
	}
	fmt.Fprintf(stdout, "Backup written to %s (%d databases).\n", args[0], len(snap.Files()))
	return 0
}
