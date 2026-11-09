package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"dendronotes/internal/store"
)

func TestAPITokens(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	base := "/api/somas/" + testSoma
	expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, benUser.ID), `{"role":"editor"}`), 200, "")
	expect(t, admin.do("POST", base+"/notes/Plan.md", `{"content":"beds"}`), 201, "")

	create := func(scope string, days int) string {
		t.Helper()
		rec := ben.do("POST", "/api/me/tokens", fmt.Sprintf(`{"name":"script %s","scope":%q,"expiresInDays":%d}`, scope, scope, days))
		expect(t, rec, 201, "")
		tok := decode[struct{ Token string }](t, rec).Token
		if !strings.HasPrefix(tok, store.TokenPrefix) || len(tok) < 40 {
			t.Fatalf("token %q", tok)
		}
		return tok
	}
	readTok := create("read", 30)
	writeTok := create("write", 0)
	with := func(tok string) func(method, path, body string, headers ...string) int {
		c := newClient(t, s)
		return func(method, path, body string, headers ...string) int {
			t.Helper()
			return c.do(method, path, body, append([]string{"Authorization", "Bearer " + tok}, headers...)...).Code
		}
	}
	rd, wr := with(readTok), with(writeTok)

	// The list never shows tokens.
	rec := ben.do("GET", "/api/me/tokens", "")
	if expect(t, rec, 200, ""); strings.Contains(rec.Body.String(), store.TokenPrefix) || len(decode[map[string][]store.APIToken](t, rec)["tokens"]) != 2 {
		t.Errorf("token list: %s", rec.Body)
	}
	for _, bad := range []string{`{"name":"","scope":"read"}`, `{"name":"x","scope":"admin"}`, `{"name":"x","scope":"read","expiresInDays":-1}`} {
		if code := ben.do("POST", "/api/me/tokens", bad).Code; code != 400 {
			t.Errorf("%s: %d", bad, code)
		}
	}

	// Reading works with both; changing only with write; managing never.
	etag := decode[map[string]any](t, admin.do("GET", base+"/notes/Plan.md", ""))["etag"].(string)
	for _, c := range []struct {
		name         string
		method, path string
		body         string
		read, write  int
	}{
		{"list somas", "GET", "/api/somas", "", 200, 200},
		{"who am I", "GET", "/api/auth/me", "", 200, 200},
		{"tree", "GET", base + "/tree", "", 200, 200},
		{"comments", "GET", base + "/comments?path=Plan.md", "", 200, 200},
		{"create note", "POST", base + "/notes/New.md", `{"content":"x"}`, 403, 201},
		{"trash note", "DELETE", base + "/entries/New.md", ``, 403, 200},
		{"share", "PUT", fmt.Sprintf("%s/members/%d", base, benUser.ID), `{"role":"viewer"}`, 403, 403},
		{"new soma", "POST", "/api/somas", `{"name":"x"}`, 403, 403},
		{"tokens", "GET", "/api/me/tokens", "", 403, 403},
		{"new token", "POST", "/api/me/tokens", `{"name":"x","scope":"read"}`, 403, 403},
		{"profile", "PATCH", "/api/me", `{"displayName":"x"}`, 403, 403},
		{"admin", "GET", "/api/admin/users", "", 403, 403},
	} {
		if got := rd(c.method, c.path, c.body); got != c.read {
			t.Errorf("%s with a read token: %d, want %d", c.name, got, c.read)
		}
		if got := wr(c.method, c.path, c.body); got != c.write {
			t.Errorf("%s with a write token: %d, want %d", c.name, got, c.write)
		}
	}
	if got := wr("PUT", base+"/notes/Plan.md", `{"content":"beds and paths"}`, "If-Match", `"`+etag+`"`); got != 200 {
		t.Errorf("saving with a write token: %d", got)
	}

	// A wrong token never falls back to the session cookie.
	c := newClient(t, s)
	c.cookie = ben.cookie
	if rec := c.do("GET", "/api/somas", "", "Authorization", "Bearer dn_wrong"); rec.Code != 401 {
		t.Errorf("wrong token with a cookie: %d", rec.Code)
	}

	// Revoked, expired, disabled: refused.
	tokens := decode[map[string][]store.APIToken](t, ben.do("GET", "/api/me/tokens", ""))["tokens"]
	expect(t, ben.do("DELETE", fmt.Sprintf("/api/me/tokens/%d", tokens[1].ID), ""), 200, "") // the read token (older)
	if got := rd("GET", "/api/somas", ""); got != 401 {
		t.Errorf("revoked token: %d", got)
	}
	expired, _, err := s.store.CreateToken(context.Background(), benUser.ID, "old", store.ScopeRead, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got := with(expired)("GET", "/api/somas", ""); got != 401 {
		t.Errorf("expired token: %d", got)
	}
	expect(t, admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", benUser.ID), `{"disabled":true}`), 200, "")
	if got := wr("GET", "/api/somas", ""); got != 401 {
		t.Errorf("token of a disabled account: %d", got)
	}
	expect(t, admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", benUser.ID), `{"disabled":false}`), 200, "")
	if got := wr("GET", "/api/somas", ""); got != 200 {
		t.Errorf("token after enabling again: %d", got)
	}

	// Admins see how many tokens there are and revoke them all.
	users := decode[struct{ Tokens map[string]int }](t, admin.do("GET", "/api/admin/users", ""))
	if users.Tokens[fmt.Sprint(benUser.ID)] != 2 {
		t.Errorf("token counts: %+v", users.Tokens)
	}
	rec = admin.do("DELETE", fmt.Sprintf("/api/admin/users/%d/tokens", benUser.ID), "")
	if expect(t, rec, 200, ""); decode[map[string]int](t, rec)["revoked"] != 2 {
		t.Errorf("revoked: %s", rec.Body)
	}
	if got := wr("GET", "/api/somas", ""); got != 401 {
		t.Errorf("token after revoking all: %d", got)
	}

	// Clients that keep sending wrong tokens are slowed down.
	bad := with("dn_" + strings.Repeat("x", 43))
	limited := false
	for range 20 {
		if code := bad("GET", "/api/somas", ""); code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("wrong tokens are not slowed down")
	}
}

// No route that manages anything is open to tokens, and read tokens never
// change anything.
func TestTokenPolicyCoversRoutes(t *testing.T) {
	s := newTestServer(t, testFS())
	for _, rt := range s.routes() {
		for _, scope := range []string{store.ScopeRead, store.ScopeWrite} {
			may := tokenMay(rt, rt.method, scope)
			switch {
			case rt.access == adminOnly || rt.access == somaOwner:
				if may {
					t.Errorf("%s %s (%s) is open to %s tokens", rt.method, rt.pattern, "managing", scope)
				}
			case strings.HasPrefix(rt.pattern, "/api/me/tokens"):
				if may {
					t.Errorf("%s %s: tokens manage tokens", rt.method, rt.pattern)
				}
			case scope == store.ScopeRead && rt.method != "GET" && rt.access != public:
				if may {
					t.Errorf("%s %s: read token may change", rt.method, rt.pattern)
				}
			case rt.access == signedIn && rt.method != "GET":
				if may {
					t.Errorf("%s %s: token changes the account", rt.method, rt.pattern)
				}
			}
		}
	}
}
