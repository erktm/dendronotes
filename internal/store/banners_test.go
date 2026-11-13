package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func bannerIDs(list []Banner) []int64 {
	ids := []int64{}
	for _, b := range list {
		ids = append(ids, b.ID)
	}
	return ids
}

func TestBanners(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	admin := user(t, s, "admin", RoleAdmin)
	ben := user(t, s, "ben", RoleUser)
	cara := user(t, s, "cara", RoleUser)
	garden, err := s.CreateGroup(ctx, "Garden")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddGroupMember(ctx, garden.ID, cara.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mk := func(b Banner) Banner {
		t.Helper()
		if b.Kind == "" {
			b.Kind = BannerInfo
		}
		if b.Audience == "" {
			b.Audience = AudienceEveryone
		}
		created, err := s.CreateBanner(ctx, b, admin.ID)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	all := mk(Banner{Message: "Update tonight", InApp: true, OnLogin: true, Dismissible: true})
	admins := mk(Banner{Message: "Admins only", InApp: true, Audience: AudienceAdmins})
	group := mk(Banner{Message: "Garden news", InApp: true, Audience: AudienceGroup, GroupID: garden.ID, Kind: BannerCritical})
	later := mk(Banner{Message: "Later", InApp: true, StartsAt: now.Add(time.Hour)})
	over := mk(Banner{Message: "Over", InApp: true, OnLogin: true, EndsAt: now.Add(-time.Minute)})
	loginOnly := mk(Banner{Message: "Sign-in only", OnLogin: true})
	_ = later
	_ = over

	if all.CreatedBy == nil || all.CreatedBy.ID != admin.ID || group.GroupName != "Garden" || all.Revision != 1 {
		t.Errorf("created: %+v / %+v", all, group)
	}
	check := func(name string, got []Banner, want ...int64) {
		t.Helper()
		ids := bannerIDs(got)
		if len(ids) != len(want) {
			t.Errorf("%s: %v, want %v", name, ids, want)
			return
		}
		for i := range ids {
			if ids[i] != want[i] {
				t.Errorf("%s: %v, want %v", name, ids, want)
				return
			}
		}
	}
	list, _ := s.ActiveBanners(ctx, ben, now)
	check("ben", list, all.ID)
	list, _ = s.ActiveBanners(ctx, cara, now)
	check("cara (critical first)", list, group.ID, all.ID)
	list, _ = s.ActiveBanners(ctx, admin, now)
	check("admin", list, all.ID, admins.ID)
	list, _ = s.LoginBanners(ctx, now)
	check("login", list, all.ID, loginOnly.ID)
	list, _ = s.ActiveBanners(ctx, ben, now.Add(2*time.Hour))
	check("ben later", list, all.ID, later.ID)

	// Closing lasts until the text changes.
	if err := s.DismissBanner(ctx, all.ID, ben.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DismissBanner(ctx, admins.ID, admin.ID); !errors.Is(err, ErrNotDismissible) {
		t.Errorf("closing a fixed banner: %v", err)
	}
	list, _ = s.ActiveBanners(ctx, ben, now)
	check("ben after closing", list)
	all.Kind = BannerWarning
	if all, err = s.UpdateBanner(ctx, all); err != nil || all.Revision != 1 {
		t.Fatalf("kind changed: %+v %v", all, err)
	}
	list, _ = s.ActiveBanners(ctx, ben, now)
	check("same text", list)
	all.Message = "Update moved to tomorrow"
	if all, err = s.UpdateBanner(ctx, all); err != nil || all.Revision != 2 {
		t.Fatalf("text changed: %+v %v", all, err)
	}
	list, _ = s.ActiveBanners(ctx, ben, now)
	check("new text", list, all.ID)

	// A deleted group takes its banners along.
	if err := s.DeleteGroup(ctx, garden.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BannerByID(ctx, group.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("group banner still there: %v", err)
	}
	if err := s.DeleteBanner(ctx, all.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBanner(ctx, all.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted twice: %v", err)
	}
	if got, _ := s.Banners(ctx); len(got) != 4 {
		t.Errorf("left: %v", bannerIDs(got))
	}
}

func TestCleanBannerText(t *testing.T) {
	for in, ok := range map[string]bool{
		"  Hello **all**\r\nline two ":        true,
		"":                                    false,
		"   ":                                 false,
		"bell\a":                              false,
		strings.Repeat("a", MaxBannerRunes+1): false,
		strings.Repeat("ä", MaxBannerRunes):   true,
	} {
		if _, got := CleanBannerText(in); got != ok {
			t.Errorf("%q: %v, want %v", in, got, ok)
		}
	}
	if s, _ := CleanBannerText(" a\r\nb "); s != "a\nb" {
		t.Errorf("cleaned %q", s)
	}
}
