package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShareLinks(t *testing.T) {
	ctx := context.Background()
	s := open(t, t.TempDir())
	anna := user(t, s, "anna", RoleUser)
	const id = "soma000001"
	if _, err := s.CreateSoma(ctx, id, "Garden", "garden", anna.ID); err != nil {
		t.Fatal(err)
	}

	// One internal link per entry, made on first use.
	a, err := s.InternalLink(ctx, id, "Beds/North.md", anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.InternalLink(ctx, id, "Beds/North.md", 0)
	if err != nil || again.ID != a.ID || a.Kind != LinkInternal || a.CreatedBy == nil || a.CreatedBy.Username != "anna" {
		t.Fatalf("internal link: %+v, again %+v, %v", a, again, err)
	}

	// Public links: the token finds them, only its hash is kept.
	token, p, err := s.CreatePublicLink(ctx, id, "Beds/North.md", anna.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || p.Kind != LinkPublic || p.ExpiresAt.IsZero() {
		t.Fatalf("public link: %q %+v", token, p)
	}
	var stored string
	if err := s.db.QueryRow(`SELECT token_hash FROM share_links WHERE id = ?`, p.ID).Scan(&stored); err != nil || stored == token {
		t.Fatalf("token stored as %q (%v)", stored, err)
	}
	got, err := s.PublicLink(ctx, token)
	if err != nil || got.ID != p.ID || got.SomaID != id || got.Expired(time.Now()) || !got.Expired(time.Now().Add(2*time.Hour)) {
		t.Fatalf("lookup: %+v %v", got, err)
	}
	if _, err := s.PublicLink(ctx, token[:42]+"x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a wrong token: %v", err)
	}

	// Links follow a renamed folder; an internal link already at the target gives way.
	if _, err := s.InternalLink(ctx, id, "Garden beds/North.md", anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveShareLinks(ctx, id, "Beds", "Garden beds"); err != nil {
		t.Fatal(err)
	}
	moved, err := s.Link(ctx, id, a.ID)
	if err != nil || moved.Path != "Garden beds/North.md" {
		t.Fatalf("after the move: %+v %v", moved, err)
	}
	if got, _ := s.PublicLink(ctx, token); got.Path != "Garden beds/North.md" {
		t.Errorf("public link after the move: %+v", got)
	}

	list, err := s.PublicLinks(ctx, id)
	if err != nil || len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("public links: %+v %v", list, err)
	}
	if _, err := s.DeletePublicLink(ctx, id, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("an internal link was revoked: %v", err)
	}
	if _, err := s.DeletePublicLink(ctx, id, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublicLink(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("a revoked link still works: %v", err)
	}

	if err := s.RemoveShareLinks(ctx, id, "Garden beds"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Link(ctx, id, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a link outlived its entry: %v", err)
	}
}
