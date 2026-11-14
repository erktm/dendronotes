package index

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Notes moved outside dendronotes are recognized by their content.
func TestSyncFindsMovesByContent(t *testing.T) {
	f := newFixture(t, map[string]string{
		"Plan.md":    "# Plan\n",
		"Twin A.md":  "same",
		"Twin B.md":  "same",
		"Empty.md":   "",
		"Notes/X.md": "# X\n",
	})
	f.sync(t)
	mv := func(from, to string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(f.dir, filepath.Dir(to)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(f.dir, from), filepath.Join(f.dir, to)); err != nil {
			t.Fatal(err)
		}
	}
	mv("Plan.md", "Archive/Plan 2026.md")
	mv("Notes", "Folder")
	mv("Twin A.md", "Twin C.md") // the same content moving twice: not told apart
	mv("Twin B.md", "Twin D.md")
	mv("Empty.md", "Blank.md") // empty notes are never paired
	ch := f.sync(t)
	want := []Move{{From: "Notes/X.md", To: "Folder/X.md"}, {From: "Plan.md", To: "Archive/Plan 2026.md"}}
	if !reflect.DeepEqual(ch.Moved, want) {
		t.Errorf("moved = %v, want %v", ch.Moved, want)
	}
	// A changed note is not a move.
	mv("Folder/X.md", "Folder/Y.md")
	f.write(t, "Folder/Y.md", "# Y, rewritten\n")
	if ch := f.sync(t); len(ch.Moved) != 0 {
		t.Errorf("moved after a rewrite = %v", ch.Moved)
	}
}
