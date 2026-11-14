package soma

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exchangeWorks reports whether the file system of the soma swaps two names
// in one step.
func exchangeWorks(t *testing.T, s *Soma) bool {
	t.Helper()
	a, b := filepath.Join(s.Dir(), ".probe-a"), filepath.Join(s.Dir(), ".probe-b")
	write(t, a, "a")
	write(t, b, "b")
	defer os.Remove(a)
	defer os.Remove(b)
	return s.exchange(".probe-a", ".probe-b") == nil
}

// Ways another program writes a file.
var (
	// inPlace truncates and writes the file, like os.WriteFile.
	inPlace = func(t *testing.T, file, content string) { write(t, file, content) }
	// replace writes a temporary file with the same permissions and renames
	// it over the file, like sync tools do.
	replace = func(t *testing.T, file, content string) {
		st, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		tmp := filepath.Join(filepath.Dir(file), ".sync-tmp")
		write(t, tmp, content)
		if err := os.Chmod(tmp, st.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, file); err != nil {
			t.Fatal(err)
		}
	}
)

// A change another program makes while a note is being saved is never
// overwritten: the save fails with a conflict, as if the change had come
// before it, and the note holds the change.
func TestWriteKeepsChangesMadeMeanwhile(t *testing.T) {
	cases := []struct {
		name     string
		exchange bool // the system swaps two names in one step
		stage    WriteStage
		change   func(t *testing.T, file, content string)
		// The note stays the file written in place, so a program still
		// writing into it keeps writing into the note.
		sameFile bool
	}{
		{"exchange/flushed/in place", true, WriteFlushed, inPlace, true},
		{"exchange/flushed/replaced", true, WriteFlushed, replace, false},
		{"exchange/replacing/in place", true, WriteReplacing, inPlace, true},
		{"exchange/replacing/replaced", true, WriteReplacing, replace, false},
		{"rename/flushed/in place", false, WriteFlushed, inPlace, true},
		{"rename/flushed/replaced", false, WriteFlushed, replace, false},
		{"rename/replacing/in place", false, WriteReplacing, inPlace, false},
		// Without an exchange a file renamed over the note between the last
		// look and our rename cannot be seen; that window is a single
		// system call, not the flush.
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, dir, _ := setup(t)
			if c.exchange && !exchangeWorks(t, s) {
				t.Skip("the file system cannot swap two names")
			}
			s.noExchange = !c.exchange
			file := filepath.Join(dir, "note.md")
			write(t, file, "base")
			if err := os.Chmod(file, 0o600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.Stat(file)
			n, err := s.ReadNote("note.md")
			if err != nil {
				t.Fatal(err)
			}
			ran := false
			s.SetWriteHook(func(p string, stage WriteStage) {
				if p == "note.md" && stage == c.stage {
					c.change(t, file, "theirs")
					ran = true
				}
			})

			_, err = s.WriteNote("note.md", []byte("ours"), n.ETag)
			if !ran {
				t.Fatal("the hook did not run")
			}
			var ce *ConflictError
			if !errors.As(err, &ce) || ce.ETag != ETag([]byte("theirs")) {
				t.Fatalf("write: %v, want a conflict with the version of the change", err)
			}
			if got, _ := os.ReadFile(file); string(got) != "theirs" {
				t.Errorf("file: %q", got)
			}
			if after, _ := os.Stat(file); c.sameFile && !os.SameFile(before, after) {
				t.Error("the note is another file now")
			}
			noTemporaryFiles(t, dir)

			// Based on the change, the write goes through.
			s.SetWriteHook(nil)
			if _, err := s.WriteNote("note.md", []byte("ours"), ce.ETag); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(file); string(got) != "ours" {
				t.Errorf("file after the second write: %q", got)
			}
			if st, _ := os.Stat(file); st.Mode().Perm() != 0o600 {
				t.Errorf("permissions: %v", st.Mode().Perm())
			}
			noTemporaryFiles(t, dir)
		})
	}
}

// Files written with an ETag (drawings) are kept the same way.
func TestWriteFileKeepsChangesMadeMeanwhile(t *testing.T) {
	s, dir, _ := setup(t)
	file := filepath.Join(dir, "Plan.drawing.svg")
	write(t, file, "<svg/>")
	s.SetWriteHook(func(p string, stage WriteStage) {
		if stage == WriteReplacing {
			inPlace(t, file, "<svg>theirs</svg>")
		}
	})
	_, _, err := s.WriteFile("Plan.drawing.svg", []byte("<svg>ours</svg>"), ETag([]byte("<svg/>")))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("write: %v", err)
	}
	if got, _ := os.ReadFile(file); string(got) != "<svg>theirs</svg>" {
		t.Errorf("file: %q", got)
	}
	noTemporaryFiles(t, dir)
}

// A note removed elsewhere while it is being saved stays removed.
func TestWriteDuringRemove(t *testing.T) {
	for _, stage := range []WriteStage{WriteFlushed, WriteReplacing} {
		s, dir, _ := setup(t)
		if stage == WriteReplacing && !exchangeWorks(t, s) {
			continue // a rename would bring it back; that window is a single system call
		}
		file := filepath.Join(dir, "note.md")
		write(t, file, "base")
		s.SetWriteHook(func(p string, at WriteStage) {
			if at == stage {
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
			}
		})
		if _, err := s.WriteNote("note.md", []byte("ours"), ETag([]byte("base"))); !errors.Is(err, ErrNotFound) {
			t.Fatalf("stage %d: write: %v", stage, err)
		}
		if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stage %d: the note is back: %v", stage, err)
		}
		noTemporaryFiles(t, dir)
	}
}

func noTemporaryFiles(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			t.Errorf("temporary file left: %s", e.Name())
		}
	}
}
