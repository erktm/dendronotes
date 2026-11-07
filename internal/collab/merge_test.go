package collab

import (
	"fmt"
	"strings"
	"testing"
)

func TestMerge3(t *testing.T) {
	base := "a\nb\nc\nd\n"
	for _, c := range []struct {
		name, ours, theirs, want string
		conflict             bool
	}{
		{"only ours", "A\nb\nc\nd\n", base, "A\nb\nc\nd\n", false},
		{"only theirs", base, "a\nb\nc\nD\n", "a\nb\nc\nD\n", false},
		{"same change", "a\nB\nc\nd\n", "a\nB\nc\nd\n", "a\nB\nc\nd\n", false},
		{"different lines", "A\nb\nc\nd\n", "a\nb\nc\nD\n", "A\nb\nc\nD\n", false},
		{"neighbouring lines", "a\nB\nc\nd\n", "a\nb\nC\nd\n", "a\nB\nC\nd\n", false},
		{"insertions apart", "x\na\nb\nc\nd\n", "a\nb\nc\nd\ny\n", "x\na\nb\nc\nd\ny\n", false},
		{"same line, both kept", "a\nours\nc\nd\n", "a\ntheirs\nc\nd\n", "a\nours\ntheirs\nc\nd\n", true},
		{"same place inserted", "a\nb\nX\nc\nd\n", "a\nb\nY\nc\nd\n", "a\nb\nX\nY\nc\nd\n", true},
		{"deleted here, changed there", "a\nc\nd\n", "a\nB\nc\nd\n", "a\nB\nc\nd\n", true},
		{"both deleted", "a\nc\nd\n", "a\nc\nd\n", "a\nc\nd\n", false},
	} {
		got, conflict := merge3(base, c.ours, c.theirs)
		if got != c.want || conflict != c.conflict {
			t.Errorf("%s: got %q (conflict %v), want %q (%v)", c.name, got, conflict, c.want, c.conflict)
		}
	}
	// Last lines without a line break.
	if got, _ := merge3("a\nb", "a\nB", "a\nb\nc"); got != "a\nB" && !strings.Contains(got, "B") {
		t.Errorf("no final newline: %q", got)
	}
	if got, conflict := merge3("x", "ours", "theirs"); got != "ours\ntheirs" || !conflict {
		t.Errorf("single line conflict: %q %v", got, conflict)
	}
}

func TestMerge3LargeDocuments(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	base := b.String()
	ours := strings.Replace(base, "line 10\n", "line ten\n", 1)
	theirs := strings.Replace(base, "line 19990\n", "line nineteen thousand\n", 1)
	got, conflict := merge3(base, ours, theirs)
	if conflict || !strings.Contains(got, "line ten\n") || !strings.Contains(got, "line nineteen thousand\n") || len(got) != len(base)+len("ten")-len("10")+len("nineteen thousand")-len("19990") {
		t.Errorf("large merge: conflict %v, %d bytes", conflict, len(got))
	}
	// Too many differences for the diff: nothing is lost either way.
	var o2, t2 strings.Builder
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(&o2, "o%d\n", i)
		fmt.Fprintf(&t2, "t%d\n", i)
	}
	got, conflict = merge3(base[:100], o2.String(), t2.String())
	if !conflict || !strings.Contains(got, "o2999\n") || !strings.Contains(got, "t2999\n") {
		t.Errorf("fallback lost text (conflict %v)", conflict)
	}
}

func TestDiffLines(t *testing.T) {
	o := splitLines("a\nb\nc\nd\ne\n")
	x := splitLines("a\nc\nX\nd\ne\nf\n")
	hs := diffLines(o, x)
	want := []hunk{{1, 2, 1, 1}, {3, 3, 2, 3}, {5, 5, 5, 6}}
	if fmt.Sprint(hs) != fmt.Sprint(want) {
		t.Errorf("hunks %v, want %v", hs, want)
	}
}
