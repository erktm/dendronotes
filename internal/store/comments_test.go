package store

import (
	"context"
	"slices"
	"testing"
)

func TestCommentsAndNotifications(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	anna := user(t, s, "anna", RoleUser)
	ben := user(t, s, "ben", RoleUser)
	cara := user(t, s, "cara", RoleUser)
	if _, err := s.CreateSoma(ctx, "soma000001", "Garden", "garden", anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMember(ctx, "soma000001", ben.ID, "viewer"); err != nil {
		t.Fatal(err)
	}

	root, err := s.AddComment(ctx, Comment{SomaID: "soma000001", Path: "Beds.md", AuthorID: anna.ID, Body: "Bed 3?", Quote: "bed 3", Prefix: "go into ", Suffix: "."})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := s.AddComment(ctx, Comment{SomaID: "soma000001", Path: "Beds.md", ThreadID: root.ID, AuthorID: ben.ID, Body: "Yes"})
	if err != nil || reply.Root() != root.ID {
		t.Fatalf("reply: %+v %v", reply, err)
	}
	if authors, _ := s.ThreadAuthors(ctx, "soma000001", root.ID); !slices.Equal(authors, []int64{anna.ID, ben.ID}) {
		t.Errorf("thread authors: %v", authors)
	}
	if c, err := s.EditComment(ctx, "soma000001", reply.ID, "Yes, bed 3"); err != nil || c.Body != "Yes, bed 3" || c.EditedAt.IsZero() {
		t.Errorf("edit: %+v %v", c, err)
	}
	if c, err := s.ResolveThread(ctx, "soma000001", root.ID, ben.ID); err != nil || c.ResolvedBy != ben.ID {
		t.Errorf("resolve: %+v %v", c, err)
	}
	if _, err := s.ResolveThread(ctx, "soma000001", reply.ID, ben.ID); err != ErrNotFound {
		t.Errorf("resolving a reply: %v", err)
	}
	if c, _ := s.ResolveThread(ctx, "soma000001", root.ID, 0); !c.ResolvedAt.IsZero() {
		t.Errorf("reopen: %+v", c)
	}

	// Notifications: only about comments that exist, in somas the account may open.
	if err := s.Notify(ctx, NotifyMention, []int64{ben.ID, cara.ID}, root, anna.ID); err != nil {
		t.Fatal(err)
	}
	list, unread, err := s.Notifications(ctx, ben.ID, 10)
	if err != nil || unread != 1 || len(list) != 1 || list[0].SomaName != "Garden" || list[0].Actor == nil || list[0].Actor.ID != anna.ID || list[0].ThreadID != root.ID {
		t.Fatalf("ben's notifications: %+v %d %v", list, unread, err)
	}
	if list, _, _ := s.Notifications(ctx, cara.ID, 10); len(list) != 0 {
		t.Errorf("cara has no access but sees %+v", list)
	}
	if err := s.MarkNotificationsRead(ctx, ben.ID, nil); err != nil {
		t.Fatal(err)
	}
	if list, unread, _ := s.Notifications(ctx, ben.ID, 10); unread != 0 || !list[0].Read {
		t.Errorf("after reading: %+v %d", list, unread)
	}

	// Comments follow moves; the notification links too.
	if err := s.MoveComments(ctx, "soma000001", "Beds.md", "Garden/Beds.md"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Comments(ctx, "soma000001", "Garden/Beds.md"); len(got) != 2 {
		t.Errorf("after move: %+v", got)
	}
	if list, _, _ := s.Notifications(ctx, ben.ID, 10); list[0].Path != "Garden/Beds.md" {
		t.Errorf("notification after move: %+v", list)
	}
	if err := s.MoveComments(ctx, "soma000001", "Garden", ".trash/Garden"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Comments(ctx, "soma000001", ".trash/Garden/Beds.md"); len(got) != 2 {
		t.Errorf("after moving the folder: %+v", got)
	}

	// People to mention: enabled accounts with access.
	people, err := s.SomaPeople(ctx, "soma000001", "", 10)
	if err != nil || len(people) != 2 || people[0].Avatar == nil {
		t.Errorf("people: %+v %v", people, err)
	}
	if people, _ := s.SomaPeople(ctx, "soma000001", "b", 10); len(people) != 1 || people[0].Username != "ben" {
		t.Errorf("people starting with b: %+v", people)
	}

	// Deleting the first comment deletes the thread and its notifications.
	if err := s.DeleteComment(ctx, "soma000001", root.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Comments(ctx, "soma000001", ".trash/Garden/Beds.md"); len(got) != 0 {
		t.Errorf("thread after deleting its start: %+v", got)
	}
	if list, _, _ := s.Notifications(ctx, ben.ID, 10); len(list) != 0 {
		t.Errorf("notification about a deleted comment: %+v", list)
	}
	if err := s.DeleteComment(ctx, "soma000001", root.ID); err != ErrNotFound {
		t.Errorf("deleting twice: %v", err)
	}

	c, _ := s.AddComment(ctx, Comment{SomaID: "soma000001", Path: "A/B.md", AuthorID: anna.ID, Body: "x"})
	if err := s.RemoveComments(ctx, "soma000001", "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommentByID(ctx, "soma000001", c.ID); err != ErrNotFound {
		t.Errorf("removed comment: %v", err)
	}
}
