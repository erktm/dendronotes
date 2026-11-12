package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"dendronotes/internal/config"
	"dendronotes/internal/mail"
	"dendronotes/internal/store"
)

// fakeRelay records what would have gone to the SMTP relay.
type fakeRelay struct {
	mu   sync.Mutex
	sent []sentMail
	fail error
}

type sentMail struct {
	to, subject, text string
}

func (f *fakeRelay) Send(_ context.Context, to string, raw []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	_, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	part, err := multipart.NewReader(msg.Body, params["boundary"]).NextRawPart()
	if err != nil {
		return err
	}
	text, _ := io.ReadAll(quotedprintable.NewReader(part))
	f.sent = append(f.sent, sentMail{to: to, subject: subject, text: string(text)})
	return nil
}

func (f *fakeRelay) setFail(err error) {
	f.mu.Lock()
	f.fail = err
	f.mu.Unlock()
}

// take returns what was sent since the last call.
func (f *fakeRelay) take() []sentMail {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.sent
	f.sent = nil
	return out
}

func newMailServer(t *testing.T) (*Server, *fakeRelay) {
	t.Helper()
	cfg := testConfig(t)
	cfg.Auth.PublicURL = "https://notes.example.org"
	from, _ := netmail.ParseAddress("dendronotes <no-reply@example.org>")
	cfg.Mail = config.Mail{Host: "smtp.invalid", Port: 587, TLS: "starttls", From: from.String(), FromAddr: from,
		DigestDelay: 10 * time.Minute}
	s := newTestServerWith(t, testFS(), cfg)
	relay := &fakeRelay{}
	s.mail.setSender(relay)
	return s, relay
}

// delivered hands the outbox to the relay and returns what it got.
func delivered(s *Server, relay *fakeRelay) []sentMail {
	s.deliverMails(context.Background())
	return relay.take()
}

// newMailUser makes an account with an email address and signs it in.
func newMailUser(t *testing.T, s *Server, admin *client, name string) (*client, store.User) {
	t.Helper()
	c, u := newUser(t, s, admin, name)
	expect(t, admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", u.ID), fmt.Sprintf(`{"email":"%s@example.org"}`, name)), 200, "")
	u, _ = s.store.UserByID(context.Background(), u.ID)
	return c, u
}

func subjects(mails []sentMail) []string {
	var out []string
	for _, m := range mails {
		out = append(out, m.to+": "+m.subject)
	}
	return out
}

func TestMailSharing(t *testing.T) {
	s, relay := newMailServer(t)
	admin := newClient(t, s)
	admin.login()
	_, ben := newMailUser(t, s, admin, "ben")
	cara, caraUser := newMailUser(t, s, admin, "cara")
	_, dave := newMailUser(t, s, admin, "dave")
	_, noAddr := newUser(t, s, admin, "nora")
	delivered(s, relay)

	base := "/api/somas/" + testSoma
	share := func(u store.User, role string) {
		t.Helper()
		expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, u.ID), fmt.Sprintf(`{"role":%q}`, role)), 200, "")
	}
	share(ben, "editor")
	got := delivered(s, relay)
	if len(got) != 1 || got[0].to != "ben@example.org" || got[0].subject != "admin shared “Test” with you" ||
		!strings.Contains(got[0].text, "editor (also rename, move and delete)") ||
		!strings.Contains(got[0].text, "https://notes.example.org/s/"+testSoma) {
		t.Fatalf("share: %+v", got)
	}
	// A new role is no new access; no address, no email.
	share(ben, "viewer")
	share(noAddr, "viewer")
	if got := delivered(s, relay); len(got) != 0 {
		t.Errorf("unexpected: %v", subjects(got))
	}

	// A group: only those who had no access yet.
	g := decode[store.Group](t, admin.do("POST", "/api/admin/groups", `{"name":"Garden"}`))
	for _, u := range []store.User{ben, caraUser} {
		expect(t, admin.do("PUT", fmt.Sprintf("/api/admin/groups/%d/members/%d", g.ID, u.ID), ""), 200, "")
	}
	expect(t, admin.do("PUT", fmt.Sprintf("%s/groups/%d", base, g.ID), `{"role":"writer"}`), 200, "")
	if got := subjects(delivered(s, relay)); len(got) != 1 || got[0] != "cara@example.org: admin shared “Test” with you" {
		t.Errorf("group: %v", got)
	}

	// Everyone never sends email; nor does a share for someone who turned
	// it off.
	var everyone store.Group
	for _, gr := range decode[struct{ Groups []store.Group }](t, admin.do("GET", "/api/admin/groups", "")).Groups {
		if gr.Everyone {
			everyone = gr
		}
	}
	expect(t, admin.do("PUT", fmt.Sprintf("%s/groups/%d", base, everyone.ID), `{"role":"viewer"}`), 200, "")
	if got := delivered(s, relay); len(got) != 0 {
		t.Errorf("everyone: %v", subjects(got))
	}
	expect(t, admin.do("DELETE", fmt.Sprintf("%s/groups/%d", base, everyone.ID), ""), 200, "")
	expect(t, cara.do("PATCH", "/api/me", `{"mailShares":false}`), 200, "")
	expect(t, admin.do("DELETE", fmt.Sprintf("%s/groups/%d", base, g.ID), ""), 200, "")
	share(caraUser, "viewer")
	share(dave, "viewer")
	if got := subjects(delivered(s, relay)); len(got) != 1 || !strings.HasPrefix(got[0], "dave@example.org:") {
		t.Errorf("opted out: %v", got)
	}
}

func TestMailDigest(t *testing.T) {
	s, relay := newMailServer(t)
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newMailUser(t, s, admin, "ben")
	cara, caraUser := newMailUser(t, s, admin, "cara")
	base := "/api/somas/" + testSoma
	for _, u := range []store.User{benUser, caraUser} {
		expect(t, admin.do("PUT", fmt.Sprintf("%s/members/%d", base, u.ID), `{"role":"writer"}`), 200, "")
	}
	expect(t, admin.do("POST", base+"/notes/Garden/Beds.md", `{"content":"Tomatoes go into bed 3."}`), 201, "")
	delivered(s, relay)
	comment := func(c *client, body string) {
		t.Helper()
		expect(t, c.do("POST", base+"/comments", fmt.Sprintf(`{"path":"Garden/Beds.md","body":%q,"quote":"bed 3"}`, body)), 201, "")
	}
	ctx := context.Background()
	later := func() time.Time { return time.Now().Add(11 * time.Minute) }

	comment(cara, "Why bed 3, @ben? <b>sunny</b>")
	s.sendDigests(ctx, time.Now()) // too early
	if got := delivered(s, relay); len(got) != 0 {
		t.Fatalf("too early: %v", subjects(got))
	}
	s.sendDigests(ctx, later())
	got := delivered(s, relay)
	if len(got) != 1 || got[0].to != "ben@example.org" || got[0].subject != "cara mentioned you in Beds" ||
		!strings.Contains(got[0].text, "Why bed 3, @ben? <b>sunny</b>") ||
		!strings.Contains(got[0].text, "https://notes.example.org/s/"+testSoma+"/n/Garden/Beds.md") {
		t.Fatalf("digest: %+v", got)
	}
	s.sendDigests(ctx, later()) // once only
	if got := delivered(s, relay); len(got) != 0 {
		t.Errorf("twice: %v", subjects(got))
	}

	// Two at once become one email; read ones none.
	comment(cara, "@ben one")
	comment(cara, "@ben two")
	s.sendDigests(ctx, later())
	if got := subjects(delivered(s, relay)); len(got) != 1 || got[0] != "ben@example.org: 2 new comments for you" {
		t.Errorf("bundle: %v", got)
	}
	comment(cara, "@ben read it")
	expect(t, ben.do("POST", "/api/me/notifications/read", `{}`), 200, "")
	s.sendDigests(ctx, later())
	if got := delivered(s, relay); len(got) != 0 {
		t.Errorf("read: %v", subjects(got))
	}
	expect(t, ben.do("PATCH", "/api/me", `{"mailComments":false}`), 200, "")
	comment(cara, "@ben off")
	s.sendDigests(ctx, later())
	if got := delivered(s, relay); len(got) != 0 {
		t.Errorf("turned off: %v", subjects(got))
	}
}

var linkToken = regexp.MustCompile(`/reset-password#([A-Za-z0-9_-]+)`)

func tokenIn(t *testing.T, m sentMail) string {
	t.Helper()
	match := linkToken.FindStringSubmatch(m.text)
	if match == nil {
		t.Fatalf("no link in %q", m.text)
	}
	return match[1]
}

func TestPasswordReset(t *testing.T) {
	s, relay := newMailServer(t)
	admin := newClient(t, s)
	admin.login()
	ben, _ := newMailUser(t, s, admin, "ben")
	delivered(s, relay)
	anon := newClient(t, s)

	if m := decode[map[string]any](t, anon.do("GET", "/api/auth/methods", "")); m["passwordReset"] != true {
		t.Errorf("methods: %v", m)
	}
	// The same answer with and without an account; a second request soon
	// after sends nothing.
	for _, login := range []string{"ben@example.org", "nobody", "ben"} {
		expect(t, anon.do("POST", "/api/auth/password/forgot", fmt.Sprintf(`{"login":%q}`, login)), 202, "")
	}
	got := delivered(s, relay)
	if len(got) != 1 || got[0].to != "ben@example.org" || got[0].subject != "Reset your dendronotes password" {
		t.Fatalf("reset mail: %v", subjects(got))
	}
	token := tokenIn(t, got[0])

	expect(t, anon.do("POST", "/api/auth/password/reset", fmt.Sprintf(`{"token":%q,"password":"short"}`, token)), 400, "weak_password")
	expect(t, anon.do("POST", "/api/auth/password/reset", `{"token":"wrong","password":"a long new password"}`), 400, "invalid_token")
	rec := anon.do("POST", "/api/auth/password/reset", fmt.Sprintf(`{"token":%q,"password":"a long new password"}`, token))
	expect(t, rec, 200, "")
	expect(t, anon.do("POST", "/api/auth/password/reset", fmt.Sprintf(`{"token":%q,"password":"another long password"}`, token)), 400, "invalid_token")
	// The old session ended; the new password works.
	expect(t, ben.do("GET", "/api/auth/me", ""), 401, "")
	ben.loginAs("ben", "a long new password")
	if got := subjects(delivered(s, relay)); len(got) != 1 || got[0] != "ben@example.org: Your dendronotes password was changed" {
		t.Errorf("notice: %v", got)
	}

	// Many requests from one address are limited.
	limited := false
	for range 20 {
		if rec := anon.do("POST", "/api/auth/password/forgot", `{"login":"x"}`); rec.Code == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("forgot password is not rate limited")
	}
}

func TestWelcomeMail(t *testing.T) {
	s, relay := newMailServer(t)
	admin := newClient(t, s)
	admin.login()
	expect(t, admin.do("POST", "/api/admin/users", `{"username":"fin","role":"user","sendWelcome":true}`), 400, "no_email")
	expect(t, admin.do("POST", "/api/admin/users", `{"username":"fin","role":"user","email":"fin@example.org"}`), 400, "weak_password")
	expect(t, admin.do("POST", "/api/admin/users", `{"username":"fin","role":"user","email":"fin@example.org","sendWelcome":true}`), 201, "")
	got := delivered(s, relay)
	if len(got) != 1 || got[0].subject != "Welcome to dendronotes" || !strings.Contains(got[0].text, "Your user name: fin") {
		t.Fatalf("welcome: %+v", got)
	}
	token := tokenIn(t, got[0])
	// No password yet: no way in with one.
	fin := newClient(t, s)
	expect(t, fin.do("POST", "/api/auth/login", `{"username":"fin","password":"anything at all"}`), 401, "invalid_credentials")
	expect(t, fin.do("POST", "/api/auth/password/reset", fmt.Sprintf(`{"token":%q,"password":"fins own password"}`, token)), 200, "")
	fin.loginAs("fin", "fins own password")
	if got := delivered(s, relay); len(got) != 1 || !strings.Contains(got[0].text, "A password was chosen for your account fin") {
		t.Errorf("notice: %+v", got)
	}

	// With a password the email only says hello.
	expect(t, admin.do("POST", "/api/admin/users", `{"username":"gus","role":"user","email":"gus@example.org","password":"gus password long","sendWelcome":true}`), 201, "")
	if got := delivered(s, relay); len(got) != 1 || strings.Contains(got[0].text, "reset-password") || !strings.Contains(got[0].text, "Sign in with the password you got from admin") {
		t.Errorf("with password: %+v", got)
	}
}

func TestSecurityNotices(t *testing.T) {
	s, relay := newMailServer(t)
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newMailUser(t, s, admin, "ben")
	delivered(s, relay)

	expect(t, ben.do("POST", "/api/me/password", `{"current":"ben password long","new":"ben new password"}`), 200, "")
	expect(t, ben.do("POST", "/api/me/tokens", `{"name":"backup <script>","scope":"read"}`), 201, "")
	expect(t, ben.do("PATCH", "/api/me", `{"email":"ben@new.example.org"}`), 200, "")
	expect(t, admin.do("POST", fmt.Sprintf("/api/admin/users/%d/password", benUser.ID), `{"password":"set by the admin"}`), 200, "")
	got := delivered(s, relay)
	want := []string{
		"ben@example.org: Your dendronotes password was changed",
		"ben@example.org: A new API token for your dendronotes account",
		"ben@example.org: Your dendronotes email address was changed",
		"ben@new.example.org: Your dendronotes password was changed",
	}
	if fmt.Sprint(subjects(got)) != fmt.Sprint(want) {
		t.Fatalf("notices:\n%v\nwant\n%v", subjects(got), want)
	}
	if !strings.Contains(got[2].text, "changed to ben@new.example.org") || !strings.Contains(got[3].text, "admin, an administrator") {
		t.Errorf("texts: %q / %q", got[2].text, got[3].text)
	}
}

func TestMailRetries(t *testing.T) {
	s, relay := newMailServer(t)
	admin := newClient(t, s)
	admin.login()
	ctx := context.Background()
	relay.setFail(errors.New("connection refused"))
	me := decode[struct{ User store.User }](t, admin.do("GET", "/api/auth/me", "")).User
	expect(t, admin.do("PATCH", fmt.Sprintf("/api/admin/users/%d", me.ID), `{"email":"admin@example.org"}`), 200, "")
	// The first and only message in the outbox.
	s.queueMailTo(ctx, "ben@example.org", "Ben", mail.KindTest, mail.Data{Host: "x"})
	const onlyMail = 1
	s.deliverMails(ctx)
	waiting, lastErr, _ := s.store.MailQueue(ctx)
	if waiting != 1 || lastErr == "" {
		t.Fatalf("queue %d %q", waiting, lastErr)
	}
	if due, _ := s.store.DueMails(ctx, 10); len(due) != 0 {
		t.Fatal("retried at once")
	}
	status := decode[map[string]any](t, admin.do("GET", "/api/admin/server", ""))["mail"].(map[string]any)
	if status["enabled"] != true || status["waiting"] != float64(1) || status["lastError"] == "" {
		t.Errorf("status %v", status)
	}

	// A permanent refusal drops the message.
	if err := s.store.MailRetry(ctx, onlyMail, time.Now().Add(-time.Second), "x"); err != nil {
		t.Fatal(err)
	}
	relay.setFail(&mail.PermanentError{Err: errors.New("550 no such user")})
	s.deliverMails(ctx)
	if waiting, _, _ := s.store.MailQueue(ctx); waiting != 0 {
		t.Errorf("still waiting: %d", waiting)
	}

	// The test email reports the relay's answer at once.
	expect(t, admin.do("POST", "/api/admin/mail/test", ""), 502, "mail_failed")
	relay.setFail(nil)
	expect(t, admin.do("POST", "/api/admin/mail/test", ""), 200, "")
	if got := subjects(relay.take()); len(got) != 1 || got[0] != "admin@example.org: dendronotes test email" {
		t.Errorf("test mail: %v", got)
	}
}

func TestNoMailWithoutSMTP(t *testing.T) {
	s := newTestServer(t, testFS())
	admin := newClient(t, s)
	admin.login()
	if m := decode[map[string]any](t, admin.do("GET", "/api/auth/methods", "")); m["passwordReset"] != false {
		t.Errorf("methods: %v", m)
	}
	if me := decode[map[string]any](t, admin.do("GET", "/api/auth/me", "")); me["mail"] != false {
		t.Errorf("me: %v", me)
	}
	expect(t, admin.do("POST", "/api/auth/password/forgot", `{"login":"admin"}`), 404, "not_available")
	expect(t, admin.do("POST", "/api/admin/mail/test", ""), 404, "not_available")
	expect(t, admin.do("POST", "/api/admin/users", `{"username":"fin","role":"user","email":"fin@example.org","sendWelcome":true}`), 400, "no_email")
}
