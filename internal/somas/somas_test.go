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

	if err := m.Delete(ctx, s, u.ID); err != nil {
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
	if err := m.Delete(context.Background(), s, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Old.md")); err != nil {
		t.Error("deleting an imported soma must leave its folder alone")
	}
}

// A deleted soma comes back with its files, owner and shares; purging it
// removes its files from the trash and nothing else.
func TestRestoreAndPurgeDeletedSoma(t *testing.T) {
	m, st, u := setup(t)
	ctx := context.Background()
	ben, err := st.CreateUser(ctx, store.User{Username: "ben", Role: store.RoleUser, Source: store.SourceLocal})
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Create(ctx, "Garden", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Path(s), "Plan.md"), []byte("beds"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := st.SetMember(ctx, s.ID, ben.ID, "writer"); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(ctx, s, ben.ID); err != nil {
		t.Fatal(err)
	}
	// Someone takes the folder name meanwhile.
	if _, err := m.Create(ctx, "Garden", u.ID); err != nil {
		t.Fatal(err)
	}
	list, err := st.DeletedSomas(ctx)
	if err != nil || len(list) != 1 || list[0].Owner == nil || list[0].Owner.ID != u.ID || list[0].Members != 1 ||
		list[0].DeletedBy == nil || list[0].DeletedBy.ID != ben.ID || !strings.HasPrefix(list[0].Folder, TrashDir+string(filepath.Separator)) {
		t.Fatalf("deleted somas: %+v %v", list, err)
	}

	back, err := m.Restore(ctx, s.ID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != s.ID || back.Folder != "garden-2" || back.Name != "Garden" {
		t.Errorf("restored: %+v", back)
	}
	if data, err := os.ReadFile(filepath.Join(m.Path(back), "Plan.md")); err != nil || string(data) != "beds" {
		t.Errorf("files after restore: %q %v", data, err)
	}
	if role, _ := st.EffectiveRole(ctx, s.ID, ben.ID); role != "writer" {
		t.Errorf("share after restore: %q", role)
	}
	if list, _ := st.DeletedSomas(ctx); len(list) != 0 {
		t.Errorf("still listed as deleted: %+v", list)
	}

	// Delete again and purge: the files go, the other soma stays.
	if err := m.Delete(ctx, back, u.ID); err != nil {
		t.Fatal(err)
	}
	d, _ := st.DeletedSomaByID(ctx, s.ID)
	if err := m.Purge(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(m.opts.SomasDir, d.Folder)); !os.IsNotExist(err) {
		t.Errorf("purged files remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(m.opts.SomasDir, "garden")); err != nil {
		t.Errorf("the other soma's folder: %v", err)
	}
	if _, err := m.Restore(ctx, s.ID, u.ID); err == nil {
		t.Error("a purged soma came back")
	}
	// A record pointing outside the trash is refused.
	if _, err := m.trashFolder("../etc"); err == nil {
		t.Error("folder outside the trash accepted")
	}
	if _, err := m.trashFolder(filepath.Join(TrashDir, "..", "garden")); err == nil {
		t.Error("escaping folder accepted")
	}
}
