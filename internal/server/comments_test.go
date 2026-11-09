package server

import (
	"fmt"
	"strings"
	"testing"

	"dendronotes/internal/store"
)

type commentList struct {
	Comments []store.Comment
	People   map[string]store.Person
}

type notificationList struct {
	Notifications []store.Notification
	Unread        int
}

func TestComments(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")    // viewer
	cara, caraUser := newUser(t, s, admin, "cara") // writer
	dave, daveUser := newUser(t, s, admin, "dave") // editor
	eve, _ := newUser(t, s, admin, "eve")          // no access
	base := "/api/somas/" + testSoma
	for u, role := range map[int64]string{benUser.ID: "viewer", caraUser.ID: "writer", daveUser.ID: "editor"} {
		expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, u), fmt.Sprintf(`{"role":%q}`, role)), 200, "")
	}
	expect(t, admin.do("POST", base+"/notes/Beds.md", `{"content":"Tomatoes go into bed 3."}`), 201, "")
	add := func(c *client, body string) *store.Comment {
		t.Helper()
		rec := c.do("POST", base+"/comments", body)
		if rec.Code != 201 {
			t.Fatalf("comment %s: %d %s", body, rec.Code, rec.Body)
		}
		c2 := decode[store.Comment](t, rec)
		return &c2
	}
	notes := func(c *client) notificationList {
		t.Helper()
		rec := c.do("GET", "/api/me/notifications", "")
		expect(t, rec, 200, "")
		return decode[notificationList](t, rec)
	}

	// Readers comment too; @mentions notify people with access only.
	root := add(ben, `{"path":"Beds.md","body":"Why bed 3, @cara? cc @eve @nobody @ben","quote":"bed 3","prefix":"go into ","suffix":"."}`)
	if root.ThreadID != 0 || root.Quote != "bed 3" || root.AuthorID != benUser.ID {
		t.Fatalf("thread: %+v", root)
	}
	if n := notes(cara); n.Unread != 1 || n.Notifications[0].Kind != "mention" || n.Notifications[0].Actor.Username != "ben" || n.Notifications[0].Path != "Beds.md" {
		t.Errorf("cara: %+v", n)
	}
	for _, c := range []*client{eve, ben} {
		if n := notes(c); n.Unread != 0 {
			t.Errorf("notified without access or about oneself: %+v", n)
		}
	}
	expect(t, eve.do("GET", base+"/comments?path=Beds.md", ""), 404, "not_found")

	// Replies notify the others in the thread (not twice when mentioned).
	reply := add(cara, fmt.Sprintf(`{"threadId":%d,"body":"Sun all day. @ben"}`, root.ID))
	if reply.Path != "Beds.md" || reply.ThreadID != root.ID || reply.Quote != "" {
		t.Errorf("reply: %+v", reply)
	}
	if n := notes(ben); n.Unread != 1 || n.Notifications[0].Kind != "mention" {
		t.Errorf("ben after the reply: %+v", n)
	}
	add(dave, fmt.Sprintf(`{"threadId":%d,"body":"Agreed."}`, root.ID))
	if n := notes(ben); n.Unread != 2 || n.Notifications[0].Kind != "reply" {
		t.Errorf("ben after dave's reply: %+v", n)
	}
	rec := ben.do("POST", "/api/me/notifications/read", `{"ids":[`+fmt.Sprint(notes(ben).Notifications[0].ID)+`]}`)
	if expect(t, rec, 200, ""); decode[map[string]int](t, rec)["unread"] != 1 {
		t.Errorf("after reading one: %s", rec.Body)
	}
	expect(t, ben.do("POST", "/api/me/notifications/read", `{}`), 200, "")
	if n := notes(ben); n.Unread != 0 {
		t.Errorf("after reading all: %+v", n)
	}

	// The list comes with the people in it.
	rec = ben.do("GET", base+"/comments?path=Beds.md", "")
	expect(t, rec, 200, "")
	list := decode[commentList](t, rec)
	if len(list.Comments) != 3 || list.People[fmt.Sprint(caraUser.ID)].Username != "cara" {
		t.Fatalf("comments: %+v", list)
	}

	// Rights: own comments only; done by writers; others' deleted by editors.
	edit := fmt.Sprintf("%s/comments/%d", base, reply.ID)
	expect(t, ben.do("PATCH", edit, `{"body":"mine now"}`), 403, "not_author")
	expect(t, cara.do("PATCH", edit, `{"body":" "}`), 400, "invalid_comment")
	expect(t, cara.do("PATCH", edit, `{"body":"Sun all day long."}`), 200, "")
	resolve := fmt.Sprintf("%s/comments/%d/resolve", base, root.ID)
	expect(t, ben.do("POST", resolve, `{"resolved":true}`), 403, "forbidden")
	expect(t, cara.do("POST", fmt.Sprintf("%s/comments/%d/resolve", base, reply.ID), `{"resolved":true}`), 400, "not_a_thread")
	rec = cara.do("POST", resolve, `{"resolved":true}`)
	if expect(t, rec, 200, ""); decode[store.Comment](t, rec).ResolvedBy != caraUser.ID {
		t.Errorf("resolved: %s", rec.Body)
	}
	expect(t, ben.do("DELETE", edit, ``), 403, "forbidden")
	expect(t, dave.do("DELETE", edit, ``), 200, "")

	// Bad input.
	for _, body := range []string{`{"path":"Beds.md","body":""}`, `{"path":"Beds.md","body":"` + strings.Repeat("x", 5001) + `"}`,
		`{"path":"Beds.md","body":"a\u0007b"}`, `{"path":"Beds.md","body":"x","prefix":"` + strings.Repeat("p", 65) + `"}`} {
		expect(t, ben.do("POST", base+"/comments", body), 400, "invalid_comment")
	}
	expect(t, ben.do("POST", base+"/comments", `{"path":"Missing.md","body":"x"}`), 404, "not_found")
	expect(t, ben.do("POST", base+"/comments", `{"threadId":99999,"body":"x"}`), 404, "not_found")

	// People to mention.
	rec = ben.do("GET", base+"/people?q=ca", "")
	if expect(t, rec, 200, ""); len(decode[map[string][]store.Person](t, rec)["people"]) != 1 {
		t.Errorf("people: %s", rec.Body)
	}

	// Comments follow the note: renamed, trashed (and hidden), restored.
	expect(t, admin.do("POST", base+"/move", `{"from":"Beds.md","to":"Garden.md"}`), 200, "")
	if l := decode[commentList](t, ben.do("GET", base+"/comments?path=Garden.md", "")); len(l.Comments) != 2 {
		t.Errorf("after rename: %+v", l.Comments)
	}
	if n := notes(cara); n.Notifications[0].Path != "Garden.md" {
		t.Errorf("notification after rename: %+v", n.Notifications[0])
	}
	expect(t, admin.do("DELETE", base+"/entries/Garden.md", ``), 200, "")
	if l := decode[commentList](t, ben.do("GET", base+"/comments?path=Garden.md", "")); len(l.Comments) != 0 {
		t.Errorf("trashed note: %+v", l.Comments)
	}
	expect(t, ben.do("GET", base+"/comments?path=.trash/Garden.md", ""), 400, "invalid_path")
	expect(t, ben.do("POST", fmt.Sprintf("%s/comments/%d/resolve", base, root.ID), `{"resolved":false}`), 403, "forbidden")
	expect(t, cara.do("POST", fmt.Sprintf("%s/comments/%d/resolve", base, root.ID), `{"resolved":false}`), 404, "not_found")
	if n := notes(cara); len(n.Notifications) != 0 {
		t.Errorf("notifications about a trashed note: %+v", n)
	}
	expect(t, admin.do("POST", base+"/trash/Garden.md", ``), 200, "")
	if l := decode[commentList](t, ben.do("GET", base+"/comments?path=Garden.md", "")); len(l.Comments) != 2 {
		t.Errorf("restored note: %+v", l.Comments)
	}
	if n := notes(cara); len(n.Notifications) != 2 || n.Notifications[0].Path != "Garden.md" {
		t.Errorf("notifications after restoring: %+v", n)
	}
	// Deleted for good: gone with their notifications.
	expect(t, admin.do("DELETE", base+"/entries/Garden.md", ``), 200, "")
	expect(t, admin.do("DELETE", base+"/trash", ``), 200, "")
	if n := notes(cara); len(n.Notifications) != 0 {
		t.Errorf("notifications about purged comments: %+v", n)
	}
}

func TestMentions(t *testing.T) {
	for in, want := range map[string]string{
		"@ben":                      "ben",
		"hi @ben, and @cara.":       "ben,cara",
		"mail me@example.org":       "",
		"@erik@corp.example please": "erik@corp.example",
		"@a @ben @BEN":              "ben",
		"(@dora-lee)":               "dora-lee",
		"@@x":                       "",
		"@über_alles":               "über_alles",
	} {
		if got := strings.Join(mentions(in), ","); got != want {
			t.Errorf("mentions(%q) = %q, want %q", in, got, want)
		}
	}
}
