package relink

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"dendronotes/internal/soma"
)

// plan runs Plan over an in-memory soma and returns the new contents by path.
func plan(t *testing.T, files map[string]string, moves map[string]string) map[string]string {
	t.Helper()
	read := func(p string) (string, string, error) {
		c, ok := files[p]
		if !ok {
			return "", "", errors.New("not found")
		}
		return c, soma.ETag([]byte(c)), nil
	}
	notes := slices.Sorted(maps.Keys(files))
	edits, err := Plan(read, notes, notes, moves)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range edits {
		if e.Synapses == 0 {
			t.Errorf("edit for %s without synapses", e.Path)
		}
		out[e.Path] = e.Content
	}
	return out
}

func TestRename(t *testing.T) {
	files := map[string]string{
		"Projects/Garden plan.md": "# Plan",
		"Journal/Garden log.md": "Follow the [[Garden plan|plan]].\n" +
			"Also ![[garden plan#Tasks]] and [plan](../Projects/Garden%20plan.md#tasks).\n" +
			"```\n[[Garden plan]] in code\n```\n" +
			"[[Missing]] stays, [[Projects/Garden plan.md]] keeps its form.",
		"Unrelated.md": "[[Journal/Garden log]]",
	}
	got := plan(t, files, map[string]string{"Projects/Garden plan.md": "Projects/Garden plan 2026.md"})
	want := map[string]string{
		"Journal/Garden log.md": "Follow the [[Garden plan 2026|plan]].\n" +
			"Also ![[Garden plan 2026#Tasks]] and [plan](../Projects/Garden%20plan%202026.md#tasks).\n" +
			"```\n[[Garden plan]] in code\n```\n" +
			"[[Missing]] stays, [[Projects/Garden plan 2026.md]] keeps its form.",
	}
	if !maps.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestMoveKeepsWorkingLinksAndFixesRelativeOnes(t *testing.T) {
	files := map[string]string{
		"Soil types.md":           "Soil",
		"Index.md":                "[[Soil types]] and [soil](Soil%20types.md)",
		"Projects/Garden plan.md": "See [log](../Journal/Garden%20log.md) and [[Soil types]].",
		"Journal/Garden log.md":   "Log",
	}
	got := plan(t, files, map[string]string{
		"Soil types.md":           "Archive/Soil types.md",
		"Projects/Garden plan.md": "Garden plan.md",
	})
	want := map[string]string{
		// [[Soil types]] still resolves by name; the Markdown link needs the folder.
		"Index.md": "[[Soil types]] and [soil](Archive/Soil%20types.md)",
		// The moved note's own relative link follows its new location.
		"Garden plan.md": "See [log](Journal/Garden%20log.md) and [[Soil types]].",
	}
	if !maps.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestMoveIntoAmbiguityUsesPath(t *testing.T) {
	files := map[string]string{
		"Plan.md":          "top",
		"Projects/Plan.md": "project",
		"Projects/x.md":    "[[Plan]] means the project plan",
	}
	got := plan(t, files, map[string]string{"Projects/Plan.md": "Archive/Plan.md"})
	if want := "[[Archive/Plan]] means the project plan"; got["Projects/x.md"] != want {
		t.Errorf("got %q, want %q", got["Projects/x.md"], want)
	}
}

func TestFolderMove(t *testing.T) {
	files := map[string]string{
		"Projects/A.md":     "[[B]] and [b](B.md) and [top](../Top.md)",
		"Projects/B.md":     "b",
		"Top.md":            "[[Projects/A]] and [a](Projects/A.md)",
		"Other/Projects.md": "[[Projects]] is a note, not the folder",
	}
	got := plan(t, files, map[string]string{
		"Projects/A.md": "Work/Projects/A.md",
		"Projects/B.md": "Work/Projects/B.md",
	})
	want := map[string]string{
		// Moving together keeps links between moved notes; links out of the folder change.
		"Work/Projects/A.md": "[[B]] and [b](B.md) and [top](../../Top.md)",
		// Path suffix [[Projects/A]] still resolves; the Markdown link does not.
		"Top.md": "[[Projects/A]] and [a](Work/Projects/A.md)",
	}
	if !maps.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestAngleBracketsAndEscaping(t *testing.T) {
	files := map[string]string{
		"Note.md":  "x",
		"Links.md": "[a](<Note.md> \"title\") and [b](Note.md)",
	}
	got := plan(t, files, map[string]string{"Note.md": "New (draft) #1.md"})
	want := "[a](<New (draft) #1.md> \"title\") and [b](New%20%28draft%29%20%231.md)"
	if got["Links.md"] != want {
		t.Errorf("got  %q\nwant %q", got["Links.md"], want)
	}
}

func TestRelPath(t *testing.T) {
	for _, c := range []struct{ dir, target, want string }{
		{"", "a.md", "a.md"},
		{"", "x/a.md", "x/a.md"},
		{"x", "a.md", "../a.md"},
		{"x/y", "x/a.md", "../a.md"},
		{"x/y", "x/y/z/a.md", "z/a.md"},
		{"a", "a.md", "../a.md"},
	} {
		if got := relPath(c.dir, c.target); got != c.want {
			t.Errorf("relPath(%q, %q) = %q, want %q", c.dir, c.target, got, c.want)
		}
	}
}
