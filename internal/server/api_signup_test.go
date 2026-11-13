package server

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"dendronotes/internal/auth"
	"dendronotes/internal/config"
)

var confirmToken = regexp.MustCompile(`/confirm-account#([A-Za-z0-9_-]+)`)

func TestSignup(t *testing.T) {
	s, relay := newMailServer(t)
	anon := newClient(t, s)
	signup := func(c *client, user, email string) int {
		t.Helper()
		return c.do("POST", "/api/auth/signup", `{"username":"`+user+`","displayName":"New Person","email":"`+email+`","password":"a long enough password"}`).Code
	}

	// Off unless DENDRONOTES_SIGNUP is on.
	if got := decode[auth.MethodsResponse](t, anon.do("GET", "/api/auth/methods", "")); got.Signup {
		t.Fatal("sign-up offered while off")
	}
	expect(t, anon.do("POST", "/api/auth/signup", `{}`), 404, "not_available")
	s.cfg.Signup = config.Signup{Enabled: true, Domains: []string{"example.org", "team.example"}}
	if got := decode[auth.MethodsResponse](t, anon.do("GET", "/api/auth/methods", "")); !got.Signup {
		t.Fatal("sign-up not offered while on")
	}

	// Checks before anything is sent.
	expect(t, anon.do("POST", "/api/auth/signup", `{"username":"x","email":"a@example.org","password":"a long enough password"}`), 400, "invalid_username")
	expect(t, anon.do("POST", "/api/auth/signup", `{"username":"newbie","email":"a@example.org","password":"short"}`), 400, "weak_password")
	expect(t, anon.do("POST", "/api/auth/signup", `{"username":"newbie","email":"a@elsewhere.example","password":"a long enough password"}`), 400, "domain_not_allowed")
	expect(t, anon.do("POST", "/api/auth/signup", `{"username":"admin","email":"a@example.org","password":"a long enough password"}`), 409, "username_taken")

	// The account exists only after the link was used.
	if code := signup(anon, "newbie", "newbie@example.org"); code != 202 {
		t.Fatalf("sign-up: %d", code)
	}
	if _, err := s.store.UserByUsername(context.Background(), "newbie"); err == nil {
		t.Fatal("the account exists before the confirmation")
	}
	mails := delivered(s, relay)
	if len(mails) != 1 || mails[0].to != "newbie@example.org" || !strings.Contains(mails[0].subject, "Confirm") {
		t.Fatalf("mails: %v", subjects(mails))
	}
	token := confirmToken.FindStringSubmatch(mails[0].text)
	if token == nil {
		t.Fatalf("no link in %q", mails[0].text)
	}
	browser := newClient(t, s)
	rec := browser.do("POST", "/api/auth/signup/confirm", `{"token":"`+token[1]+`"}`)
	expect(t, rec, 200, "")
	browser.cookie = rec.Result().Cookies()[0]
	if me := decode[auth.MeResponse](t, browser.do("GET", "/api/auth/me", "")); me.User.Username != "newbie" || me.User.Email != "newbie@example.org" {
		t.Fatalf("signed in as %+v", me.User)
	}
	somas := decode[struct {
		Somas []struct{ Name string } `json:"somas"`
	}](t, browser.do("GET", "/api/somas", ""))
	if len(somas.Somas) != 1 {
		t.Errorf("first soma: %+v", somas)
	}
	expect(t, browser.do("POST", "/api/auth/signup/confirm", `{"token":"`+token[1]+`"}`), 400, "invalid_token")

	// The same answer for an address that has an account; that address hears about it.
	if code := signup(newClient(t, s), "someone", "NEWBIE@example.org"); code != 202 {
		t.Fatalf("sign-up for a used address: %d", code)
	}
	mails = delivered(s, relay)
	if len(mails) != 1 || !strings.Contains(mails[0].subject, "already have") || !strings.Contains(mails[0].text, "newbie") {
		t.Fatalf("mails for a used address: %v", subjects(mails))
	}

	// Switching sign-up off makes waiting links useless.
	if code := signup(newClient(t, s), "later", "later@team.example"); code != 202 {
		t.Fatalf("sign-up: %d", code)
	}
	later := confirmToken.FindStringSubmatch(delivered(s, relay)[0].text)[1]
	s.cfg.Signup.Enabled = false
	expect(t, newClient(t, s).do("POST", "/api/auth/signup/confirm", `{"token":"`+later+`"}`), 404, "not_available")
}
