package sso

import (
	"testing"
	"time"
)

func TestGroupsAndClaims(t *testing.T) {
	p := &Provider{cfg: Config{UsernameClaim: "preferred_username", GroupsClaim: "realm_access.roles"}}
	c := p.claims("iss", "sub", map[string]any{
		"preferred_username": "erik", "name": "Erik", "email": "e@example.org",
		"realm_access": map[string]any{"roles": []any{"dn-admins", 7, "dn-users"}},
	})
	if c.Username != "erik" || c.Name != "Erik" || c.Email != "e@example.org" || len(c.Groups) != 2 {
		t.Errorf("claims: %+v", c)
	}
	p.cfg.GroupsClaim = "groups"
	if c := p.claims("iss", "sub", map[string]any{}); c.Groups != nil {
		t.Errorf("missing groups claim: %+v", c.Groups)
	}
	if c := p.claims("iss", "sub", map[string]any{"groups": "a, b"}); len(c.Groups) != 2 {
		t.Errorf("groups as string: %+v", c.Groups)
	}
	if !InGroup([]string{"/dn-admins"}, []string{"dn-admins"}) || !InGroup([]string{"dn-users"}, []string{"/dn-users"}) ||
		InGroup([]string{"dn-admins-old"}, []string{"dn-admins"}) || InGroup(nil, []string{"x"}) {
		t.Error("InGroup")
	}
}

func TestPendingIsSingleUse(t *testing.T) {
	p := &Provider{pending: map[string]pending{}}
	if err := p.remember("s", pending{nonce: "n", expires: farFuture}); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.take("s"); !ok {
		t.Fatal("first take failed")
	}
	if _, ok := p.take("s"); ok {
		t.Error("state usable twice")
	}
	_ = p.remember("old", pending{expires: past})
	if _, ok := p.take("old"); ok {
		t.Error("expired state accepted")
	}
}

var (
	farFuture = mustTime("2999-01-01T00:00:00Z")
	past      = mustTime("2000-01-01T00:00:00Z")
)

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
