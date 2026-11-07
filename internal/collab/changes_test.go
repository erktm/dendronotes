package collab

import (
	"encoding/json"
	"os"
	"testing"
)

// The catalog is generated with CodeMirror; web/src/lib/collab.test.ts
// checks the same cases against CodeMirror itself.
func TestChangeSetCatalog(t *testing.T) {
	raw, err := os.ReadFile("../../web/src/lib/testdata/changesets.json")
	if err != nil {
		t.Fatal(err)
	}
	var cat struct {
		Cases []struct {
			Name    string
			Doc     string
			Changes json.RawMessage
			Result  string
		}
		Invalid []struct {
			Name    string
			Doc     string
			Changes json.RawMessage
		}
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.Cases) < 50 {
		t.Fatalf("only %d cases", len(cat.Cases))
	}
	for _, c := range cat.Cases {
		ch, err := parseChanges(c.Changes)
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		got, err := ch.apply(fromString(c.Doc))
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		if toString(got) != c.Result {
			t.Errorf("%s: got %q, want %q", c.Name, toString(got), c.Result)
		}
		// The server's own change sets (merges) describe the same edit.
		for name, raw := range map[string]json.RawMessage{
			"replaceJSON": replaceJSON(fromString(c.Doc), fromString(c.Result)),
			"diffJSON":    diffJSON(c.Doc, c.Result),
		} {
			back, err := parseChanges(raw)
			if err != nil {
				t.Fatalf("%s: %s: %v (%s)", c.Name, name, err, raw)
			}
			if again, err := back.apply(fromString(c.Doc)); err != nil || toString(again) != c.Result {
				t.Errorf("%s: %s gives %q, %v", c.Name, name, toString(again), err)
			}
		}
	}
	for _, c := range cat.Invalid {
		ch, err := parseChanges(c.Changes)
		if err == nil {
			_, err = ch.apply(fromString(c.Doc))
		}
		if err == nil {
			t.Errorf("%s: accepted", c.Name)
		}
	}
}

func TestUTF8Len(t *testing.T) {
	for _, s := range []string{"", "abc", "Grüße", "日本語", "😀👩‍🌾", "a\nb"} {
		if got := utf8Len(fromString(s)); got != int64(len(s)) {
			t.Errorf("utf8Len(%q) = %d, want %d", s, got, len(s))
		}
	}
	if got := utf8Len([]uint16{0xd800}); got != 3 { // lone surrogate → U+FFFD
		t.Errorf("lone surrogate: %d", got)
	}
}

func TestNormalizeAndFormat(t *testing.T) {
	for _, c := range []struct{ in, text string }{
		{"a\nb\n", "a\nb\n"},
		{"a\r\nb\r\n", "a\nb\n"},
		{"\ufeffTitle\r\nText", "Title\nText"},
		{"old\rmac", "old\nmac"},
	} {
		text, f := normalize(c.in)
		if text != c.text {
			t.Errorf("normalize(%q) = %q", c.in, text)
		}
		if back := string(f.apply(text)); c.in != "old\rmac" && back != c.in {
			t.Errorf("format of %q writes %q", c.in, back)
		}
	}
}

// Merged changes touch only the lines that differ.
func TestDiffJSONKeepsUnchangedLines(t *testing.T) {
	got := string(diffJSON("one\ntwo\nthree\n", "one\nTWO\nthree\nfour\n"))
	if got != `[4,[4,"TWO",""],6,[0,"four",""]]` {
		t.Errorf("diffJSON: %s", got)
	}
}
