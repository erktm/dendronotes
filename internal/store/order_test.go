package store

import (
	"context"
	"reflect"
	"testing"
)

func TestFolderOrder(t *testing.T) {
	ctx := context.Background()
	s := open(t, t.TempDir())
	anna := user(t, s, "anna", RoleUser)
	if _, err := s.CreateSoma(ctx, "soma000001", "Garden", "garden", anna.ID); err != nil {
		t.Fatal(err)
	}
	const id = "soma000001"
	set := func(folder string, names ...string) {
		t.Helper()
		if err := s.SetFolderOrder(ctx, id, folder, names); err != nil {
			t.Fatal(err)
		}
	}
	get := func() map[string][]string {
		t.Helper()
		m, err := s.FolderOrders(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	set("", "Beds", "Plan.md", "Ideas.md")
	set("Beds/North", "b.md", "a.md")
	set("Other", "x.md")
	want := map[string][]string{
		"":           {"Beds", "Plan.md", "Ideas.md"},
		"Beds/North": {"b.md", "a.md"},
		"Other":      {"x.md"},
	}
	if got := get(); !reflect.DeepEqual(got, want) {
		t.Fatalf("orders = %v, want %v", got, want)
	}

	// A renamed entry keeps its place; a renamed folder takes its orders along.
	if err := s.MoveFolderOrder(ctx, id, "Beds", "Garden beds"); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveFolderOrder(ctx, id, "Plan.md", "Archive/Plan.md"); err != nil {
		t.Fatal(err)
	}
	got := get()
	if !reflect.DeepEqual(got[""], []string{"Garden beds", "Plan.md", "Ideas.md"}) {
		t.Errorf("top level after rename = %v", got[""])
	}
	if _, ok := got["Beds/North"]; ok || !reflect.DeepEqual(got["Garden beds/North"], []string{"b.md", "a.md"}) {
		t.Errorf("subfolder after rename = %v", got)
	}

	// No names forgets the folder; removing a folder its subfolders.
	set("Other")
	if err := s.RemoveFolderOrder(ctx, id, "Garden beds"); err != nil {
		t.Fatal(err)
	}
	if got := get(); len(got) != 1 || len(got[""]) != 3 {
		t.Errorf("after removing: %v", got)
	}
	if err := s.RemoveFolderOrder(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	if got := get(); len(got) != 0 {
		t.Errorf("after removing the soma's orders: %v", got)
	}
}
