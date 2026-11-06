package somas

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/soma"
	"dendronotes/internal/store"
)

func setup(t *testing.T) (*Manager, *store.Store, store.User) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(t.TempDir(), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, err := st.CreateUser(context.Background(), store.User{Username: "anna", Role: store.RoleUser, Source: store.SourceLocal})
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, Options{SomasDir: t.TempDir(), DataDir: t.TempDir(), Soma: soma.Options{MaxNoteBytes: 1 << 20}, Log: log})
	t.Cleanup(func() { m.Close() })
	return m, st, u
}

func TestCreateNamesFolders(t *testing.T) {
	m, _, u := setup(t)
	ctx := context.Background()
	var folders []string
	for _, name := range []string{"Garden Notes", "garden notes", "Café & Crème", "   ", "../../etc", "日本"} {
		s, err := m.Create(ctx, name, u.ID)
		if strings.TrimSpace(name) == "" {
			if err == nil {
				t.Error("empty name accepted")
			}
			continue
		}
		if err != nil {
			t.Fatalf("Create(%q): %v", name, err)
		}
		if st, err := os.Stat(m.Path(s)); err != nil || !st.IsDir() {
			t.Errorf("folder of %q missing: %v", name, err)
		}
		folders = append(folders, s.Folder)
	}
	want := []string{"garden-notes", "garden-notes-2", "cafe-creme", "etc", "soma"}
	if strings.Join(folders, ",") != strings.Join(want, ",") {
		t.Errorf("folders = %v, want %v", folders, want)
	}
}

func TestAcquireDeleteAndIdle(t *testing.T) {
	m, st, u := setup(t)
	ctx := context.Background()
	s, err := m.Create(ctx, "Garden", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(m.Path(s), "Plan.md"), []byte("[[Soil]]"), 0o644)

	rt, err := m.Acquire(s)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := m.Acquire(s); again != rt {
		t.Error("a second Acquire opened the soma twice")
	}
	rt.Release()
	if _, err := rt.Index.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	// In use: not closed even when "idle" by time.
	m.closeIdle(time.Now().Add(time.Hour))
	if len(m.Running()) != 1 {
		t.Fatal("a soma in use was closed")
	}
	rt.Release()
	m.closeIdle(time.Now().Add(time.Hour))
	if len(m.Running()) != 0 {
		t.Fatal("an idle soma stayed open")
	}

	if err := m.Delete(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SomaByID(ctx, s.ID); err == nil {
		t.Error("deleted soma still registered")
	}
	trashed, _ := filepath.Glob(filepath.Join(m.opts.SomasDir, TrashDir, "garden-*", "Plan.md"))
	if len(trashed) != 1 {
		t.Errorf("soma not moved to the trash: %v", trashed)
	}
	if _, err := os.Stat(m.indexDir(s.ID)); !os.IsNotExist(err) {
		t.Errorf("index of a deleted soma remains: %v", err)
	}
}

func TestImport(t *testing.T) {
	m, _, u := setup(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Old.md"), []byte("old"), 0o644)
	s, err := m.Import(context.Background(), dir, "Notes", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m.Path(s) != dir {
		t.Errorf("path of imported soma: %s", m.Path(s))
	}
	if err := m.Delete(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Old.md")); err != nil {
		t.Error("deleting an imported soma must leave its folder alone")
	}
}
