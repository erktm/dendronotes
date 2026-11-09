package history

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func open(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	h, err := Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return now }
	return h, &now
}

func reasons(t *testing.T, h *Store, p string) []string {
	t.Helper()
	list, err := h.List(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, v := range list {
		out = append(out, v.Reason)
	}
	return out
}

func content(t *testing.T, h *Store, p string, i int) string {
	t.Helper()
	list, _ := h.List(context.Background(), p)
	if i >= len(list) {
		t.Fatalf("%s has %d versions", p, len(list))
	}
	_, c, err := h.Get(context.Background(), list[i].ID)
	if err != nil {
		t.Fatal(err)
	}
	return string(c)
}

func TestSessionsAndAuthors(t *testing.T) {
	h, now := open(t)
	ctx := context.Background()
	rec := func(c string, authors []int64, reason string) {
		t.Helper()
		if err := h.Record(ctx, "Plan.md", []byte(c), authors, reason); err != nil {
			t.Fatal(err)
		}
	}
	rec("", []int64{1}, ReasonCreate)
	rec("a", []int64{1}, ReasonEdit)
	*now = now.Add(time.Minute)
	rec("ab", []int64{1}, ReasonEdit) // same session: replaces
	rec("ab", []int64{1}, ReasonEdit) // unchanged: skipped
	if got := reasons(t, h, "Plan.md"); !slices.Equal(got, []string{ReasonEdit, ReasonCreate}) {
		t.Fatalf("versions: %v", got)
	}
	if c := content(t, h, "Plan.md", 0); c != "ab" {
		t.Errorf("newest: %q", c)
	}
	rec("abc", []int64{2, 1}, ReasonEdit) // someone joined: a new version
	*now = now.Add(Session)
	rec("abcd", []int64{1, 2}, ReasonEdit) // same people, but the session is over
	rec("abcd!", nil, ReasonOutside)
	list, _ := h.List(ctx, "Plan.md")
	if len(list) != 5 || !slices.Equal(list[2].Authors, []int64{1, 2}) || list[0].Reason != ReasonOutside || list[0].Authors == nil {
		t.Fatalf("versions: %+v", list)
	}
	// A restore names the version it brought back.
	if err := h.RecordRestore(ctx, "Plan.md", []byte("ab"), []int64{2}, list[3].ID); err != nil {
		t.Fatal(err)
	}
	list, _ = h.List(ctx, "Plan.md")
	if list[0].Reason != ReasonRestore || list[0].RestoredFrom != list[4].ID || list[0].Size != 2 {
		t.Errorf("restore: %+v", list[0])
	}
}

func TestMoveRemovePrune(t *testing.T) {
	h, now := open(t)
	ctx := context.Background()
	for _, p := range []string{"Garden/Beds.md", "Garden/Seeds/List.md", "Garden.md", "Gardening/Tools.md"} {
		if err := h.Record(ctx, p, []byte(p), nil, ReasonCreate); err != nil {
			t.Fatal(err)
		}
	}
	// A folder moves with everything in it, and only that.
	if err := h.Move(ctx, "Garden", "Allotment"); err != nil {
		t.Fatal(err)
	}
	for p, n := range map[string]int{"Allotment/Beds.md": 1, "Allotment/Seeds/List.md": 1, "Garden.md": 1, "Gardening/Tools.md": 1, "Garden/Beds.md": 0} {
		if got := len(reasons(t, h, p)); got != n {
			t.Errorf("%s: %d versions, want %d", p, got, n)
		}
	}
	if c := content(t, h, "Allotment/Seeds/List.md", 0); c != "Garden/Seeds/List.md" {
		t.Errorf("content after move: %q", c)
	}
	if err := h.Remove(ctx, "Allotment"); err != nil {
		t.Fatal(err)
	}
	if n := len(reasons(t, h, "Allotment/Beds.md")) + len(reasons(t, h, "Garden.md")); n != 1 {
		t.Errorf("after remove: %d versions left of two notes", n)
	}

	// Old versions go, the newest of every note stays.
	*now = now.Add(48 * time.Hour)
	for _, c := range []string{"v1", "v2"} {
		*now = now.Add(Session)
		if err := h.Record(ctx, "Garden.md", []byte(c), nil, ReasonEdit); err != nil {
			t.Fatal(err)
		}
	}
	n, err := h.Prune(ctx, now.Add(-time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("pruned %d %v", n, err)
	}
	if got := reasons(t, h, "Garden.md"); len(got) != 2 {
		t.Errorf("Garden.md after prune: %v", got)
	}
	if got := reasons(t, h, "Gardening/Tools.md"); len(got) != 1 {
		t.Errorf("the only version of a note was pruned: %v", got)
	}

	// A backup is a database of its own.
	file := filepath.Join(t.TempDir(), "backup.db")
	if err := h.Backup(ctx, file); err != nil {
		t.Fatal(err)
	}
	b, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if got := reasons(t, b, "Garden.md"); len(got) != 2 {
		t.Errorf("backup: %v", got)
	}
}
