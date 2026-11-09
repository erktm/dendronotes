package soma

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func trashPaths(t *testing.T, s *Soma) []string {
	t.Helper()
	entries, err := s.TrashTree()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Type+":"+e.Path)
	}
	return out
}

func TestTrashRestoreAndPurge(t *testing.T) {
	s, dir, outside := setup(t)
	if got := trashPaths(t, s); len(got) != 0 {
		t.Fatalf("empty soma has a trash: %v", got)
	}
	write(t, filepath.Join(dir, "Garden", "Beds.md"), "tomatoes")
	write(t, filepath.Join(dir, "Garden", "Seeds", "List.md"), "beans")
	write(t, filepath.Join(dir, "Ideas.md"), "ideas")

	if _, err := s.Trash("Garden/Beds.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Trash("Garden"); err != nil { // the rest of the folder
		t.Fatal(err)
	}
	// The folder lands next to the note trashed before, not inside it.
	want := []string{"dir:Garden", "file:Garden/Beds.md", "dir:Garden (1)", "dir:Garden (1)/Seeds", "file:Garden (1)/Seeds/List.md"}
	if got := trashPaths(t, s); !slices.Equal(got, want) {
		t.Fatalf("trash = %v, want %v", got, want)
	}
	n, err := s.ReadTrashNote("Garden/Beds.md")
	if err != nil || n.Content != "tomatoes" || n.Path != "Garden/Beds.md" {
		t.Fatalf("preview: %+v %v", n, err)
	}

	// Back to where it was.
	to, err := s.RestoreTrash("Garden (1)", "Garden")
	if err != nil || to != "Garden" {
		t.Fatalf("restore: %q %v", to, err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "Garden", "Seeds", "List.md")); string(data) != "beans" {
		t.Errorf("restored content: %q", data)
	}
	if got := trashPaths(t, s); !slices.Equal(got, []string{"dir:Garden", "file:Garden/Beds.md"}) {
		t.Errorf("trash after restore: %v", got)
	}
	// Missing folders are created again.
	if _, err := s.Trash("Garden/Seeds/List.md"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "Garden", "Seeds")); err != nil {
		t.Fatal(err)
	}
	if to, err := s.RestoreTrash("Garden/Seeds/List.md", "Garden/Seeds/List.md"); err != nil || to != "Garden/Seeds/List.md" {
		t.Errorf("restore into a missing folder: %q %v", to, err)
	}
	// Emptied folders in the trash disappear.
	if got := trashPaths(t, s); !slices.Equal(got, []string{"dir:Garden", "file:Garden/Beds.md"}) {
		t.Errorf("trash after the second restore: %v", got)
	}
	// An occupied place gets a suffix.
	write(t, filepath.Join(dir, "Garden", "Beds.md"), "new beds")
	if to, err := s.RestoreTrash("Garden/Beds.md", "Garden/Beds.md"); err != nil || to != "Garden/Beds (1).md" {
		t.Errorf("restore onto an existing note: %q %v", to, err)
	}
	if got := trashPaths(t, s); len(got) != 0 {
		t.Errorf("trash should be empty: %v", got)
	}

	// Purge removes only inside the trash.
	if _, err := s.Trash("Ideas.md"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../Ideas.md", ".trash/Ideas.md", "/Ideas.md", "Garden/../Ideas.md", "../outside/secret.md"} {
		if err := s.PurgeTrash(bad); err == nil {
			t.Errorf("PurgeTrash(%q) accepted", bad)
		}
		if _, err := s.RestoreTrash("Ideas.md", bad); err == nil {
			t.Errorf("RestoreTrash to %q accepted", bad)
		}
	}
	if err := s.PurgeTrash("Garden/Beds.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("purging a restored entry: %v", err)
	}
	if err := s.PurgeTrash("Ideas.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.md")); err != nil {
		t.Errorf("outside file touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Garden", "Beds.md")); err != nil {
		t.Errorf("soma file touched: %v", err)
	}

	// Empty removes everything in the trash, also what Obsidian put there.
	write(t, filepath.Join(dir, TrashDir, "From Obsidian.md"), "x")
	if _, err := s.Trash("Garden/Beds (1).md"); err != nil {
		t.Fatal(err)
	}
	if err := s.EmptyTrash(); err != nil {
		t.Fatal(err)
	}
	if got := trashPaths(t, s); len(got) != 0 {
		t.Errorf("trash after emptying: %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "Garden", "Beds.md")); err != nil {
		t.Errorf("emptying touched the soma: %v", err)
	}
}

// A symlink in the trash is listed as a file and purged as a link: its
// target stays.
func TestTrashSymlink(t *testing.T) {
	s, dir, outside := setup(t)
	if err := os.MkdirAll(filepath.Join(dir, TrashDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, TrashDir, "escape")); err != nil {
		t.Fatal(err)
	}
	if got := trashPaths(t, s); !slices.Equal(got, []string{"file:escape"}) {
		t.Errorf("trash with a symlink: %v", got)
	}
	if _, err := s.ReadTrashNote("escape/secret.md"); err == nil {
		t.Error("read through a symlink out of the soma")
	}
	if err := s.PurgeTrash("escape"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.md")); err != nil {
		t.Errorf("symlink target removed: %v", err)
	}
}

// A folder deleted as a whole stays as it was: what is deleted later from a
// new folder of the same name lands next to it.
func TestTrashAvoidsDeletedFolders(t *testing.T) {
	s, dir, _ := setup(t)
	write(t, filepath.Join(dir, "Garden", "Old.md"), "old")
	if _, err := s.Trash("Garden"); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "Garden", "Seeds", "New.md"), "new")
	avoid := func(p string) bool { return p == "Garden" }
	got, err := s.TrashAvoiding("Garden/Seeds/New.md", avoid)
	if err != nil || got != ".trash/Garden (1)/Seeds/New.md" {
		t.Fatalf("trashed to %q %v", got, err)
	}
	write(t, filepath.Join(dir, "Garden", "Seeds", "Newer.md"), "newer")
	if got, err := s.TrashAvoiding("Garden/Seeds/Newer.md", avoid); err != nil || got != ".trash/Garden (2)/Seeds/Newer.md" {
		t.Errorf("second: %q %v", got, err)
	}
	if got, err := s.TrashAvoiding("Garden", avoid); err != nil || got != ".trash/Garden (3)" {
		t.Errorf("the folder itself: %q %v", got, err)
	}
}
