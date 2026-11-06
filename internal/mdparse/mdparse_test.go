package mdparse

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// shared are the cases web/src/lib/synapses.test.ts runs as well.
type shared struct {
	Parse []struct {
		Name     string    `json:"name"`
		Src      string    `json:"src"`
		Synapses []Synapse `json:"synapses"`
	} `json:"parse"`
	Resolve struct {
		Notes []string `json:"notes"`
		Cases []struct {
			Target   string  `json:"target"`
			Markdown bool    `json:"markdown"`
			From     string  `json:"from"`
			Expected *string `json:"expected"`
		} `json:"cases"`
	} `json:"resolve"`
}

func loadShared(t *testing.T) shared {
	t.Helper()
	data, err := os.ReadFile("../../web/src/lib/testdata/synapses.json")
	if err != nil {
		t.Fatal(err)
	}
	var s shared
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSharedParseCases(t *testing.T) {
	for _, c := range loadShared(t).Parse {
		t.Run(c.Name, func(t *testing.T) {
			got := Parse(c.Src).Synapses
			for i := range got {
				got[i].Start, got[i].End = 0, 0
			}
			if len(got) == 0 && len(c.Synapses) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.Synapses) {
				t.Errorf("got  %+v\nwant %+v", got, c.Synapses)
			}
		})
	}
}

func TestSharedResolveCases(t *testing.T) {
	s := loadShared(t)
	r := NewResolver(s.Resolve.Notes)
	for _, c := range s.Resolve.Cases {
		want := ""
		if c.Expected != nil {
			want = *c.Expected
		}
		if got := r.Resolve(c.Target, c.Markdown, c.From); got != want {
			t.Errorf("Resolve(%q, md=%v, from %q) = %q, want %q", c.Target, c.Markdown, c.From, got, want)
		}
	}
}

func TestMaskCodeKeepsOffsets(t *testing.T) {
	for _, src := range []string{
		"a `[[x]]` b\n```\n[[y]]\n```",
		"ü `ä` ``ö`` ```\n~~~\nß\n~~~",
		"`unclosed [[z]]",
	} {
		m := MaskCode(src)
		if len(m) != len(src) {
			t.Errorf("MaskCode(%q) changed the length: %d → %d", src, len(src), len(m))
		}
	}
	if m := MaskCode("a `[[x]]` b\n```\n[[y]]\n```"); strings.Contains(m, "[[") {
		t.Errorf("code not masked: %q", m)
	}
}

func TestSynapseOffsets(t *testing.T) {
	src := "Über [[Plan|p]] and [b](Other.md)"
	syn := Parse(src).Synapses
	if len(syn) != 2 {
		t.Fatalf("got %d synapses", len(syn))
	}
	if got := src[syn[0].Start:syn[0].End]; got != "[[Plan|p]]" {
		t.Errorf("first synapse spans %q", got)
	}
	if got := src[syn[1].Start:syn[1].End]; got != "[b](Other.md)" {
		t.Errorf("second synapse spans %q", got)
	}
}

func TestTags(t *testing.T) {
	src := strings.Join([]string{
		"---",
		"title: Plan",
		"tags: [garden, \"to-do\"]",
		"color: #ff0000",
		"---",
		"#project/garden at the start, and #Garden again (duplicate).",
		"Not tags: [[Note#Heading]], [x](#anchor), https://example.org/#frag, # Heading, #2025, a#b",
		"`#code` and #2025-plans",
		"```",
		"#fenced",
		"```",
	}, "\n")
	got := Parse(src).Tags
	want := []string{"garden", "to-do", "project/garden", "2025-plans"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %q, want %q", got, want)
	}
}

func TestFrontMatterBlockList(t *testing.T) {
	src := "---\r\naliases: x\r\ntags:\r\n  - one\r\n  - 'two'\r\nother: y\r\n---\r\nBody #three"
	fm, body := FrontMatter(src)
	if !strings.Contains(fm, "tags:") || !strings.HasPrefix(src[body:], "Body") {
		t.Fatalf("front matter %q, body %q", fm, src[body:])
	}
	if got, want := Parse(src).Tags, []string{"one", "two", "three"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tags = %q, want %q", got, want)
	}
}

func TestNoFrontMatter(t *testing.T) {
	for _, src := range []string{"", "---", "--- \nno end", "text\n---\nx\n---"} {
		if fm, body := FrontMatter(src); fm != "" || body != 0 {
			t.Errorf("FrontMatter(%q) = %q, %d", src, fm, body)
		}
	}
}

func TestNameKey(t *testing.T) {
	for in, want := range map[string]string{
		"Plan":              "plan.md",
		"Projects/Plan":     "plan.md",
		"/Café":             "café.md",
		"Notes.txt":         "notes.txt",
		"../sub/My Note.md": "my note.md",
	} {
		if got := NameKey(in); got != want {
			t.Errorf("NameKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"[[a]] `b` [c](d.md)", "```\n[[x]]", "``a`[[b]]`", "---\ntags: [x]\n---\n#y"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(MaskCode(src)) != len(src) {
			t.Fatal("MaskCode changed the length")
		}
		for _, s := range Parse(src).Synapses {
			if s.Start < 0 || s.End > len(src) || s.Start >= s.End {
				t.Fatalf("bad offsets %d..%d", s.Start, s.End)
			}
		}
	})
}
