package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPendingSignups(t *testing.T) {
	ctx := context.Background()
	s := open(t, t.TempDir())

	p := PendingSignup{Username: "anna", DisplayName: "Anna", Email: "anna@example.org", PasswordHash: "hash"}
	first, err := s.AddPendingSignup(ctx, p, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// A second sign-up for the address replaces the first.
	second, err := s.AddPendingSignup(ctx, PendingSignup{Username: "anna2", Email: "ANNA@example.org", PasswordHash: "h2"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakePendingSignup(ctx, first); !errors.Is(err, ErrNotFound) {
		t.Errorf("the replaced sign-up still works: %v", err)
	}
	got, err := s.TakePendingSignup(ctx, second)
	if err != nil || got.Username != "anna2" || got.PasswordHash != "h2" {
		t.Fatalf("taken: %+v %v", got, err)
	}
	if _, err := s.TakePendingSignup(ctx, second); !errors.Is(err, ErrNotFound) {
		t.Errorf("a token worked twice: %v", err)
	}

	// Expired tokens are refused.
	expired, err := s.AddPendingSignup(ctx, p, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakePendingSignup(ctx, expired); !errors.Is(err, ErrNotFound) {
		t.Errorf("an expired sign-up: %v", err)
	}

	anna := user(t, s, "anna", RoleUser)
	addr := "Anna@Example.org"
	if _, err := s.UpdateUser(ctx, anna.ID, UserUpdate{Email: &addr}); err != nil {
		t.Fatal(err)
	}
	if found, err := s.UsersByEmail(ctx, "anna@example.ORG"); err != nil || len(found) != 1 || found[0].ID != anna.ID {
		t.Errorf("users by email: %+v %v", found, err)
	}
}
