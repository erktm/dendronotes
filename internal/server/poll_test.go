package server

import (
	"fmt"
	"testing"
	"time"

	"dendronotes/internal/index"
	"dendronotes/internal/store"
)

// shortPolls makes polls answer and streams expire quickly.
func shortPolls(t *testing.T, wait, expiry time.Duration) {
	t.Helper()
	oldWait, oldExpiry := pollWait, pollExpiry
	pollWait, pollExpiry = wait, expiry
	t.Cleanup(func() { pollWait, pollExpiry = oldWait, oldExpiry })
}

// Long polling delivers the same events as the event stream: a hello first,
// then every change, each once, in order.
func TestPollStream(t *testing.T) {
	shortPolls(t, 300*time.Millisecond, time.Minute)
	s := newIndexedServer(t, liveFiles)
	c := newClient(t, s)
	c.login()
	base := "/api/somas/" + testSoma + "/poll"

	rec := c.do("GET", base, "")
	expect(t, rec, 200, "")
	opened := decode[pollResponse](t, rec)
	if opened.Stream == "" || len(opened.Events) != 1 || opened.Events[0].Name != "hello" {
		t.Fatalf("open: %+v", opened)
	}
	if want := fmt.Sprintf(`{"ready":true,"stream":%q}`, opened.Stream); string(opened.Events[0].Data) != want {
		t.Errorf("hello: %s", opened.Events[0].Data)
	}

	// Nothing happened: the poll waits, then answers empty.
	start := time.Now()
	rec = c.do("GET", base+"?stream="+opened.Stream+"&after=0", "")
	expect(t, rec, 200, "")
	if got := decode[pollResponse](t, rec); len(got.Events) != 0 || time.Since(start) < 250*time.Millisecond {
		t.Errorf("idle poll: %+v after %s", got, time.Since(start))
	}

	// Events published between two polls wait for the next one.
	rt := rtOf(t, s)
	rt.Publish(index.Changes{Notes: []index.NoteChange{{Path: "a.md", ETag: "1"}}})
	rt.Publish(index.Changes{Notes: []index.NoteChange{{Path: "b.md", ETag: "2"}}})
	rec = c.do("GET", base+"?stream="+opened.Stream+"&after=0", "")
	got := decode[pollResponse](t, rec)
	if len(got.Events) != 2 || got.Events[0].Name != "change" || got.Events[0].Seq != 1 || got.Events[1].Seq != 2 {
		t.Fatalf("queued events: %+v", got)
	}
	if string(got.Events[1].Data) != `{"tree":false,"notes":[{"path":"b.md","etag":"2"}],"removed":[]}` {
		t.Errorf("change data: %s", got.Events[1].Data)
	}
	// Unacknowledged events come again (an answer may get lost), acknowledged ones never.
	rec = c.do("GET", base+"?stream="+opened.Stream+"&after=1", "")
	if got := decode[pollResponse](t, rec); len(got.Events) != 1 || got.Events[0].Seq != 2 {
		t.Errorf("after 1: %+v", got)
	}

	// A waiting poll answers as soon as something happens.
	done := make(chan pollResponse, 1)
	go func() {
		done <- decode[pollResponse](t, c.do("GET", base+"?stream="+opened.Stream+"&after=2", ""))
	}()
	time.Sleep(50 * time.Millisecond)
	rt.Publish(index.Changes{Tree: true})
	select {
	case got := <-done:
		if len(got.Events) != 1 || got.Events[0].Seq != 3 {
			t.Errorf("woken poll: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiting poll did not answer")
	}
}

// A stream belongs to the account that opened it, and ends when nobody polls it.
func TestPollStreamOwnerAndExpiry(t *testing.T) {
	shortPolls(t, 100*time.Millisecond, 200*time.Millisecond)
	s := newIndexedServer(t, liveFiles)
	admin := newClient(t, s)
	admin.login()
	ben, benUser := newUser(t, s, admin, "ben")
	if err := s.store.SetMember(t.Context(), testSoma, benUser.ID, store.MemberViewer); err != nil {
		t.Fatal(err)
	}
	base := "/api/somas/" + testSoma + "/poll"
	opened := decode[pollResponse](t, admin.do("GET", base, ""))
	subscribers := rtOf(t, s).Hub.Subscribers()

	expect(t, ben.do("GET", base+"?stream="+opened.Stream, ""), 410, "stream_gone")
	expect(t, admin.do("GET", base+"?stream=unknown", ""), 410, "stream_gone")

	time.Sleep(400 * time.Millisecond)
	expect(t, admin.do("GET", base+"?stream="+opened.Stream, ""), 410, "stream_gone")
	if n := rtOf(t, s).Hub.Subscribers(); n != subscribers-1 {
		t.Errorf("subscribers after expiry: %d, before %d", n, subscribers)
	}
}

// Joining a note works with a polled stream like with an event stream.
func TestPollStreamJoinsNotes(t *testing.T) {
	shortPolls(t, 2*time.Second, time.Minute)
	s := newIndexedServer(t, liveFiles)
	c := newClient(t, s)
	c.login()
	base := "/api/somas/" + testSoma
	stream := decode[pollResponse](t, c.do("GET", base+"/poll", "")).Stream
	expect(t, c.do("POST", base+"/collab/join", fmt.Sprintf(`{"stream":%q,"path":"Soil types.md"}`, stream)), 200, "")
	expect(t, c.do("POST", base+"/collab/leave", fmt.Sprintf(`{"stream":%q,"path":"Soil types.md"}`, stream)), 200, "")
}
