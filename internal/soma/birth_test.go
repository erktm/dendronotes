package soma

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The tree reports when entries were created, not when they last changed,
// where the file system keeps birth times.
func TestTreeCreationTimes(t *testing.T) {
	s, dir, _ := setup(t)
	start := time.Now().Add(-time.Minute)
	write(t, filepath.Join(dir, "Beds", "North.md"), "# North")
	// A later change (macOS moves the birth time back along with an earlier one).
	old := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	for _, p := range []string{filepath.Join(dir, "Beds", "North.md"), filepath.Join(dir, "Beds")} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Path != "Beds" && e.Path != "Beds/North.md" {
			continue
		}
		if e.MTime != old.UnixMilli() {
			t.Fatalf("%s: mtime %d", e.Path, e.MTime)
		}
		if e.CTime == e.MTime {
			t.Skip("this file system keeps no birth times")
		}
		if got := time.UnixMilli(e.CTime); got.Before(start) || got.After(time.Now().Add(time.Minute)) {
			t.Errorf("%s: created %v, want about now", e.Path, got)
		}
	}
}
