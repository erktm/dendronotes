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
	set := func(folder string, o FolderOrder) {
		t.Helper()
		if err := s.SetFolderOrder(ctx, id, folder, o); err != nil {
			t.Fatal(err)
		}
	}
	get := func() map[string]FolderOrder {
		t.Helper()
		m, err := s.FolderOrders(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	set("", FolderOrder{Mode: SortManual, Names: []string{"Beds", "Plan.md", "Ideas.md"}})
	set("Beds", FolderOrder{Mode: SortModified, Names: []string{"ignored"}})
	set("Beds/North", FolderOrder{Mode: SortManual, Names: []string{"b.md", "a.md"}})
	set("Other", FolderOrder{Mode: SortCreated})
	if err := s.SetFolderOrder(ctx, id, "x", FolderOrder{Mode: "random"}); err == nil {
		t.Error("an unknown mode was stored")
	}
	want := map[string]FolderOrder{
		"":           {Mode: SortManual, Names: []string{"Beds", "Plan.md", "Ideas.md"}},
		"Beds":       {Mode: SortModified, Names: []string{}},
		"Beds/North": {Mode: SortManual, Names: []string{"b.md", "a.md"}},
		"Other":      {Mode: SortCreated, Names: []string{}},
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
	if o := got[""]; !reflect.DeepEqual(o.Names, []string{"Garden beds", "Plan.md", "Ideas.md"}) {
		t.Errorf("top level after rename = %v", o.Names)
	}
	if _, ok := got["Beds"]; ok {
		t.Error("the old folder kept its order")
	}
	if o := got["Garden beds/North"]; !reflect.DeepEqual(o.Names, []string{"b.md", "a.md"}) {
		t.Errorf("subfolder after rename = %v", o)
	}
	if o := got["Garden beds"]; o.Mode != SortModified {
		t.Errorf("folder after rename = %v", o)
	}

	// Sorting by name again forgets the folder; removing a folder its subfolders.
	set("Other", FolderOrder{Mode: SortName})
	if err := s.RemoveFolderOrder(ctx, id, "Garden beds"); err != nil {
		t.Fatal(err)
	}
	if got := get(); len(got) != 1 || got[""].Mode != SortManual {
		t.Errorf("after removing: %v", got)
	}
	if err := s.RemoveFolderOrder(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	if got := get(); len(got) != 0 {
		t.Errorf("after removing the soma's orders: %v", got)
	}
}
