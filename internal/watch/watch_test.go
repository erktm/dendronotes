package watch

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func newWatcher(t *testing.T) (string, <-chan Batch, *Watcher) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := New(Options{
		Dir: dir,
		Dirs: func() ([]string, error) {
			var dirs []string
			err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil || !d.IsDir() || p == dir {
					return err
				}
				if strings.HasPrefix(d.Name(), ".") {
					return fs.SkipDir
				}
				rel, _ := filepath.Rel(dir, p)
				dirs = append(dirs, filepath.ToSlash(rel))
				return nil
			})
			return dirs, err
		},
		Skip: func(rel string) bool {
			for _, seg := range strings.Split(rel, "/") {
				if strings.HasPrefix(seg, ".") {
					return true
				}
			}
			return false
		},
		Debounce: 50 * time.Millisecond,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan Batch, 16)
	done := make(chan struct{})
	go func() { w.Run(ctx, func(b Batch) { out <- b }); close(done) }()
	t.Cleanup(func() { cancel(); <-done; w.Close() })
	return dir, out, w
}

// collect gathers the paths of all batches until none arrives for a while.
func collect(t *testing.T, out <-chan Batch) []string {
	t.Helper()
	seen := map[string]bool{}
	timeout := time.After(5 * time.Second)
	quiet := 400 * time.Millisecond
	for {
		select {
		case b := <-out:
			for _, p := range b.Paths {
				seen[p] = true
			}
		case <-time.After(quiet):
			if len(seen) > 0 {
				var paths []string
				for p := range seen {
					paths = append(paths, p)
				}
				slices.Sort(paths)
				return paths
			}
		case <-timeout:
			t.Fatal("no events")
		}
	}
}

func TestReportsChangesAndSkipsHidden(t *testing.T) {
	dir, out, _ := newWatcher(t)
	write(t, filepath.Join(dir, ".hidden.md"), "x")
	write(t, filepath.Join(dir, "Note.md"), "x")
	write(t, filepath.Join(dir, "Projects", "Plan.md"), "x")
	got := collect(t, out)
	if !slices.Equal(got, []string{"Note.md", "Projects/Plan.md"}) {
		t.Errorf("paths = %v", got)
	}
}

func TestWatchesNewFolders(t *testing.T) {
	dir, out, w := newWatcher(t)
	if err := os.Mkdir(filepath.Join(dir, "New"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := collect(t, out); !slices.Contains(got, "New") {
		t.Fatalf("folder creation not reported: %v", got)
	}
	w.UpdateDirs()
	write(t, filepath.Join(dir, "New", "Idea.md"), "x")
	if got := collect(t, out); !slices.Contains(got, "New/Idea.md") {
		t.Errorf("file in new folder not reported: %v", got)
	}
}

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
