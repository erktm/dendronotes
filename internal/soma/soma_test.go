package soma

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// setup creates <tmp>/soma and a sibling <tmp>/outside/secret.md that must
// never be reachable through the soma.
func setup(t *testing.T) (s *Soma, dir, outside string) {
	t.Helper()
	base := t.TempDir()
	dir = filepath.Join(base, "soma")
	outside = filepath.Join(base, "outside")
	for _, d := range []string{dir, outside} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("TOP SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, Options{MaxNoteBytes: 1 << 20, Ignore: []string{"node_modules"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir, outside
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not available:", err)
	}
}

func TestCleanPath(t *testing.T) {
	ok := []string{"a.md", "Projects/Idea.md", "ä ö ü/日本.md", "a b/c:d.md", "x/y/z"}
	for _, p := range ok {
		if _, err := CleanPath(p); err != nil {
			t.Errorf("CleanPath(%q) rejected: %v", p, err)
		}
	}
	bad := []string{
		"", "/", "/etc/passwd", "a/", "a//b", "..", "../a", "a/../b", "a/..", ".", "./a",
		".git/config", "a/.trash/b", ".trash", "a\\b", "a\x00b", "a\nb", "\x7f",
		"\xff\xfe", strings.Repeat("a", 256), strings.Repeat("a/", 2049) + "a",
	}
	for _, p := range bad {
		if _, err := CleanPath(p); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("CleanPath(%q) = %v, want ErrInvalidPath", p, err)
		}
	}
}

func TestValidateName(t *testing.T) {
	ok := map[string]string{
		"Idea.md":         "Idea.md",
		"Meeting 12.md":   "Meeting 12.md",
		"Café.md":         "Café.md",
		"Café.md":        "Café.md", // NFD input is normalized to NFC
		"console.md":      "console.md",
		"2026-11-05 Plan": "2026-11-05 Plan",
	}
	for in, want := range ok {
		got, err := ValidateName(in)
		if err != nil || got != want {
			t.Errorf("ValidateName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{"", ".hidden", "a/b", "a:b", "a*b", "a?b", `a"b`, "a<b", "a>b", "a|b", " lead", "trail ", "dot.", "CON", "nul.md", "Lpt1.txt", "a\tb"}
	for _, in := range bad {
		if _, err := ValidateName(in); !errors.Is(err, ErrInvalidName) {
			t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", in, err)
		}
	}
}

// FuzzCleanPath checks the core invariant: every accepted path is local,
// has no hidden or dot segments, and cannot reach the outside secret.
func FuzzCleanPath(f *testing.F) {
	for _, s := range []string{"a.md", "../outside/secret.md", "a/../../outside/secret.md", "%2e%2e/x", ".trash/x", "a\\..\\b", "x/./y"} {
		f.Add(s)
	}
	s, _, _ := setupFuzz(f)
	f.Fuzz(func(t *testing.T, p string) {
		clean, err := CleanPath(p)
		if err != nil {
			return
		}
		if !filepath.IsLocal(clean) {
			t.Fatalf("accepted non-local path %q", p)
		}
		for seg := range strings.SplitSeq(clean, "/") {
			if seg == "" || strings.HasPrefix(seg, ".") {
				t.Fatalf("accepted bad segment in %q", p)
			}
		}
		if n, err := s.ReadNote(clean); err == nil && strings.Contains(n.Content, "TOP SECRET") {
			t.Fatalf("read outside secret via %q", p)
		}
	})
}

func setupFuzz(f *testing.F) (*Soma, string, string) {
	base := f.TempDir()
	dir := filepath.Join(base, "soma")
	outside := filepath.Join(base, "outside")
	os.Mkdir(dir, 0o755)
	os.Mkdir(outside, 0o755)
	os.WriteFile(filepath.Join(outside, "secret.md"), []byte("TOP SECRET"), 0o644)
	os.Symlink("../outside", filepath.Join(dir, "link"))
	os.Symlink("../outside/secret.md", filepath.Join(dir, "s.md"))
	s, err := Open(dir, Options{})
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { s.Close() })
	return s, dir, outside
}

func TestSymlinkEscapesAreBlocked(t *testing.T) {
	s, dir, outside := setup(t)
	write(t, filepath.Join(dir, "real.md"), "inside")
	symlink(t, "../outside/secret.md", filepath.Join(dir, "escape.md"))           // relative, escaping
	symlink(t, filepath.Join(outside, "secret.md"), filepath.Join(dir, "abs.md")) // absolute
	symlink(t, "../outside", filepath.Join(dir, "linkdir"))                       // directory, escaping
	symlink(t, "real.md", filepath.Join(dir, "alias.md"))                         // inside, allowed read-only

	for _, p := range []string{"escape.md", "abs.md", "linkdir/secret.md"} {
		n, err := s.ReadNote(p)
		if err == nil || strings.Contains(n.Content, "TOP SECRET") {
			t.Errorf("ReadNote(%q) escaped the soma: %q %v", p, n.Content, err)
		}
		if _, err := s.WriteNote(p, []byte("pwned"), ""); err == nil {
			t.Errorf("WriteNote(%q) succeeded", p)
		}
	}
	if _, err := s.CreateNote("linkdir/new.md", []byte("pwned")); err == nil {
		t.Error("CreateNote through an escaping directory symlink succeeded")
	}
	if _, err := s.Mkdir("linkdir/sub"); err == nil {
		t.Error("Mkdir through an escaping directory symlink succeeded")
	}
	if _, err := s.Move("real.md", "linkdir/real.md"); err == nil {
		t.Error("Move through an escaping directory symlink succeeded")
	}
	if got, _ := os.ReadFile(filepath.Join(outside, "secret.md")); string(got) != "TOP SECRET" {
		t.Fatalf("outside file was modified: %q", got)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Fatalf("files were created outside the soma: %v", entries)
	}

	// The in-soma alias is readable but read-only.
	n, err := s.ReadNote("alias.md")
	if err != nil || n.Content != "inside" || !n.ReadOnly {
		t.Errorf("alias.md: %+v %v", n, err)
	}
	if _, err := s.WriteNote("alias.md", []byte("x"), n.ETag); !errors.Is(err, ErrReadOnly) {
		t.Errorf("writing through a symlink: %v, want ErrReadOnly", err)
	}

	tree, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}
	paths := treePaths(tree)
	for _, hidden := range []string{"escape.md", "abs.md", "linkdir", "linkdir/secret.md"} {
		if slices.Contains(paths, hidden) {
			t.Errorf("tree lists escaping entry %q", hidden)
		}
	}
	if !slices.Contains(paths, "alias.md") {
		t.Error("tree misses the in-soma alias")
	}
}

func treePaths(entries []Entry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func TestTreeHidesIgnoredEntries(t *testing.T) {
	s, dir, _ := setup(t)
	write(t, filepath.Join(dir, "Projects", "Idea.md"), "x")
	write(t, filepath.Join(dir, "image.png"), "png")
	write(t, filepath.Join(dir, ".obsidian", "app.json"), "{}")
	write(t, filepath.Join(dir, ".trash", "old.md"), "x")
	write(t, filepath.Join(dir, "Projects", ".hidden.md"), "x")
	write(t, filepath.Join(dir, "node_modules", "x.md"), "x")
	write(t, filepath.Join(dir, "bad\\name.md"), "x")

	tree, err := s.Tree()
	if err != nil {
		t.Fatal(err)
	}
	got := treePaths(tree)
	want := []string{"Projects", "Projects/Idea.md", "image.png"}
	if !slices.Equal(got, want) {
		t.Fatalf("tree = %v, want %v", got, want)
	}
	if tree[0].Type != "dir" || tree[1].Type != "file" || tree[1].Size != 1 {
		t.Errorf("unexpected entry details: %+v", tree)
	}
}

func TestReadWriteWithETag(t *testing.T) {
	s, dir, _ := setup(t)
	write(t, filepath.Join(dir, "note.md"), "v1")
	if err := os.Chmod(filepath.Join(dir, "note.md"), 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := s.ReadNote("note.md")
	if err != nil || n.Content != "v1" || n.ETag == "" {
		t.Fatalf("read: %+v %v", n, err)
	}
	n2, err := s.WriteNote("note.md", []byte("v2"), n.ETag)
	if err != nil || n2.ETag == n.ETag {
		t.Fatalf("write: %+v %v", n2, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "note.md")); string(got) != "v2" {
		t.Fatalf("file content %q", got)
	}
	if st, _ := os.Stat(filepath.Join(dir, "note.md")); st.Mode().Perm() != 0o600 {
		t.Errorf("permissions not preserved: %v", st.Mode().Perm())
	}

	// Stale ETag → conflict carrying the current version.
	_, err = s.WriteNote("note.md", []byte("v3"), n.ETag)
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.ETag != n2.ETag || !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write: %v", err)
	}

	// External change → conflict as well.
	write(t, filepath.Join(dir, "note.md"), "changed elsewhere")
	if _, err := s.WriteNote("note.md", []byte("v4"), n2.ETag); !errors.Is(err, ErrConflict) {
		t.Fatalf("write after external change: %v", err)
	}

	// No temporary files left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			t.Errorf("temporary file left: %s", e.Name())
		}
	}
}

func TestReadNoteRejections(t *testing.T) {
	s, dir, _ := setup(t)
	write(t, filepath.Join(dir, "image.png"), "png")
	write(t, filepath.Join(dir, "latin1.md"), "caf\xe9")
	write(t, filepath.Join(dir, "big.md"), strings.Repeat("x", (1<<20)+1))
	if err := os.Mkdir(filepath.Join(dir, "folder.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]error{
		"image.png":  ErrNotNote,
		"latin1.md":  ErrNotNote,
		"big.md":     ErrTooLarge,
		"missing.md": ErrNotFound,
		"folder.md":  ErrIsDir,
		"../x.md":    ErrInvalidPath,
	}
	for p, want := range cases {
		if _, err := s.ReadNote(p); !errors.Is(err, want) {
			t.Errorf("ReadNote(%q) = %v, want %v", p, err, want)
		}
	}
	if _, err := s.WriteNote("note.md", make([]byte, (1<<20)+1), ""); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversized write: %v", err)
	}
}

func TestCreateAndMkdir(t *testing.T) {
	s, dir, _ := setup(t)
	n, err := s.CreateNote("New/Deep/Idea.md", []byte("# Idea"))
	if err != nil || n.Path != "New/Deep/Idea.md" {
		t.Fatalf("create: %+v %v", n, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "New", "Deep", "Idea.md")); string(got) != "# Idea" {
		t.Fatalf("content %q", got)
	}
	if _, err := s.CreateNote("New/Deep/Idea.md", nil); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate create: %v", err)
	}
	if _, err := s.CreateNote("bad:name.md", nil); !errors.Is(err, ErrInvalidName) {
		t.Errorf("invalid name: %v", err)
	}
	if _, err := s.CreateNote("script.js", nil); !errors.Is(err, ErrNotNote) {
		t.Errorf("non-note create: %v", err)
	}
	if n, err := s.CreateNote("Café.md", nil); err != nil || n.Path != "Café.md" {
		t.Errorf("NFC normalization on create: %+v %v", n, err)
	}

	if _, err := s.Mkdir("Archive/2026"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := s.Mkdir("Archive/2026"); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate mkdir: %v", err)
	}
}

func TestMove(t *testing.T) {
	s, dir, _ := setup(t)
	write(t, filepath.Join(dir, "a.md"), "a")
	write(t, filepath.Join(dir, "b.md"), "b")
	write(t, filepath.Join(dir, "Folder", "inner.md"), "i")
	if err := os.Mkdir(filepath.Join(dir, "Target"), 0o755); err != nil {
		t.Fatal(err)
	}

	if to, err := s.Move("a.md", "Target/a.md"); err != nil || to != "Target/a.md" {
		t.Fatalf("move file: %q %v", to, err)
	}
	if _, err := s.Move("Folder", "Target/Folder"); err != nil {
		t.Fatalf("move folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Target", "Folder", "inner.md")); err != nil {
		t.Fatal("folder content not moved")
	}
	if _, err := s.Move("b.md", "Target/a.md"); !errors.Is(err, ErrExists) {
		t.Errorf("move onto existing: %v", err)
	}
	if _, err := s.Move("Target", "Target/Sub/Target"); !errors.Is(err, ErrMoveIntoSelf) {
		t.Errorf("move into itself: %v", err)
	}
	if _, err := s.Move("b.md", "Missing/b.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("move into missing folder: %v", err)
	}
	if _, err := s.Move("b.md", "Target/a.md/b.md"); err == nil {
		t.Error("move below a file succeeded")
	}
	if _, err := s.Move("b.md", ".trash/b.md"); err == nil {
		t.Error("move into a hidden folder succeeded")
	}
	if to, err := s.Move("b.md", "B.md"); err != nil || to != "B.md" {
		t.Errorf("case-only rename: %q %v", to, err)
	}
}

func TestTrash(t *testing.T) {
	s, dir, _ := setup(t)
	write(t, filepath.Join(dir, "Projects", "Idea.md"), "one")
	got, err := s.Trash("Projects/Idea.md")
	if err != nil || got != ".trash/Projects/Idea.md" {
		t.Fatalf("trash: %q %v", got, err)
	}
	write(t, filepath.Join(dir, "Projects", "Idea.md"), "two")
	got, err = s.Trash("Projects/Idea.md")
	if err != nil || got != ".trash/Projects/Idea (1).md" {
		t.Fatalf("trash with clash: %q %v", got, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".trash", "Projects", "Idea.md")); string(b) != "one" {
		t.Errorf("first trashed version lost: %q", b)
	}
	if _, err := s.Trash("Projects"); err != nil {
		t.Fatalf("trash folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".trash", "Projects (1)")); err != nil {
		t.Errorf("trashed folder clash not resolved: %v", err)
	}
	if _, err := s.Trash("missing.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("trash missing: %v", err)
	}
	if _, err := s.Trash(".trash"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("trashing the trash: %v", err)
	}
}

func TestCheck(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "soma")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Check(); err != nil {
		t.Fatalf("fresh soma: %v", err)
	}
	// Remove the directory: the open handle still refers to the deleted
	// directory, but the path does not – Check must report that.
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(); err == nil {
		t.Fatal("removed soma was not detected")
	}
	// A new, different directory at the same path is not a valid soma either.
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(); !errors.Is(err, ErrReplaced) {
		t.Fatalf("replaced soma: expected ErrReplaced, got %v", err)
	}
}

func TestOpen(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing"), Options{}); err == nil {
		t.Fatal("expected error for a missing directory")
	}
	if _, err := Open(t.TempDir(), Options{Ignore: []string{"[bad"}}); err == nil {
		t.Fatal("expected error for an invalid ignore pattern")
	}
}

func TestStat(t *testing.T) {
	s, dir, outside := setup(t)
	write(t, filepath.Join(dir, "Notes", "a.md"), "hello")
	write(t, filepath.Join(dir, "node_modules", "x.md"), "ignored")
	write(t, filepath.Join(dir, ".git", "config.md"), "hidden")
	symlink(t, "Notes/a.md", filepath.Join(dir, "link.md"))                          // inside: read-only file
	symlink(t, filepath.Join(outside, "secret.md"), filepath.Join(dir, "escape.md")) // absolute: never followed
	symlink(t, "Notes", filepath.Join(dir, "dirlink"))                               // folders are not followed

	if e, err := s.Stat("Notes/a.md"); err != nil || e.Type != "file" || e.Size != 5 || e.ReadOnly {
		t.Errorf("file: %+v, %v", e, err)
	}
	if e, err := s.Stat("Notes"); err != nil || e.Type != "dir" {
		t.Errorf("dir: %+v, %v", e, err)
	}
	if e, err := s.Stat("link.md"); err != nil || e.Type != "file" || !e.ReadOnly {
		t.Errorf("symlink inside: %+v, %v", e, err)
	}
	for _, p := range []string{"escape.md", "dirlink", "node_modules/x.md", ".git/config.md", "missing.md", "../outside/secret.md"} {
		if _, err := s.Stat(p); err == nil {
			t.Errorf("Stat(%q) succeeded", p)
		}
	}
}

func TestCreateFile(t *testing.T) {
	s, dir, _ := setup(t)
	allowAll := func([]byte) error { return nil }
	png := "\x89PNG\r\n\x1a\nrest of the image"

	e, err := s.CreateFile("Projects/photo.png", strings.NewReader(png), 1<<20, false, allowAll)
	if err != nil || e.Path != "Projects/photo.png" || e.Size != int64(len(png)) {
		t.Fatalf("CreateFile: %+v, %v", e, err)
	}
	if _, err := s.CreateFile("Projects/photo.png", strings.NewReader(png), 1<<20, false, allowAll); !errors.Is(err, ErrExists) {
		t.Errorf("existing name: %v", err)
	}
	if e, err := s.CreateFile("Projects/photo.png", strings.NewReader(png), 1<<20, true, allowAll); err != nil || e.Path != "Projects/photo (1).png" {
		t.Errorf("unique name: %+v, %v", e, err)
	}

	// Too large: nothing remains, not even the temporary file.
	if _, err := s.CreateFile("big.bin", strings.NewReader(strings.Repeat("x", 2000)), 1000, false, allowAll); !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}
	// Exactly the limit is fine.
	if _, err := s.CreateFile("limit.bin", strings.NewReader(strings.Repeat("x", 1000)), 1000, false, allowAll); err != nil {
		t.Errorf("at the limit: %v", err)
	}
	// The check sees the first bytes and can refuse.
	var seen string
	refuse := func(head []byte) error { seen = string(head); return errors.New("no") }
	if _, err := s.CreateFile("evil.png", strings.NewReader("<html>"), 1000, false, refuse); !errors.Is(err, ErrType) || seen != "<html>" {
		t.Errorf("refused type: %v (saw %q)", err, seen)
	}
	for _, p := range []string{"../outside.png", ".hidden/x.png", "node_modules/x.png", "a/.x.png", "/abs.png"} {
		if _, err := s.CreateFile(p, strings.NewReader(png), 1000, false, allowAll); err == nil {
			t.Errorf("CreateFile(%q) succeeded", p)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{"Projects", "limit.bin"}; !slices.Equal(names, want) {
		t.Errorf("soma contains %v, want %v (no temporary or refused files)", names, want)
	}
}

func TestOpenFile(t *testing.T) {
	s, dir, outside := setup(t)
	write(t, filepath.Join(dir, "a.png"), "png")
	write(t, filepath.Join(dir, ".git", "x"), "hidden")
	symlink(t, filepath.Join(outside, "secret.md"), filepath.Join(dir, "escape.md"))
	f, e, err := s.OpenFile("a.png")
	if err != nil || e.Size != 3 {
		t.Fatalf("OpenFile: %+v %v", e, err)
	}
	f.Close()
	for _, p := range []string{".git/x", "escape.md", "missing.png", "../outside/secret.md"} {
		if f, _, err := s.OpenFile(p); err == nil {
			f.Close()
			t.Errorf("OpenFile(%q) succeeded", p)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OpenFile("folder"); !errors.Is(err, ErrIsDir) {
		t.Errorf("folder: %v", err)
	}
}
