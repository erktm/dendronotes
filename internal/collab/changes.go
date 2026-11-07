package collab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
)

// The editor (CodeMirror) counts positions in UTF-16 code units and keeps
// lines separated by "\n". A document here is therefore a []uint16 of the
// text with normalized line breaks – exactly what every client holds.

// maxParts bounds the sections of one change set.
const maxParts = 200_000

// part is one section of a change set: keep n units, or replace n units
// with ins (an empty ins deletes).
type part struct {
	n       int
	replace bool
	ins     []uint16
}

// changes is a parsed CodeMirror ChangeSet.
type changes []part

// parseChanges reads the JSON form of a ChangeSet (ChangeSet.toJSON):
// a number keeps that many units; [n] deletes n units; [n, "line", …]
// replaces n units with the lines joined by "\n".
func parseChanges(raw json.RawMessage) (changes, error) {
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, errors.New("change set: not an array")
	}
	if len(parts) > maxParts {
		return nil, errors.New("change set: too many sections")
	}
	out := make(changes, 0, len(parts))
	for _, p := range parts {
		p = bytes.TrimSpace(p)
		if len(p) > 0 && p[0] == '[' {
			var items []json.RawMessage
			if err := json.Unmarshal(p, &items); err != nil || len(items) == 0 {
				return nil, errors.New("change set: invalid replacement")
			}
			n, err := length(items[0])
			if err != nil {
				return nil, err
			}
			var ins []uint16
			for i, item := range items[1:] {
				var line string
				if err := json.Unmarshal(item, &line); err != nil {
					return nil, errors.New("change set: inserted lines must be strings")
				}
				if strings.ContainsAny(line, "\r\n") {
					return nil, errors.New("change set: line break inside a line")
				}
				if i > 0 {
					ins = append(ins, '\n')
				}
				ins = append(ins, utf16.Encode([]rune(line))...)
			}
			out = append(out, part{n: n, replace: true, ins: ins})
			continue
		}
		n, err := length(p)
		if err != nil {
			return nil, err
		}
		out = append(out, part{n: n})
	}
	return out, nil
}

func length(raw json.RawMessage) (int, error) {
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil || f < 0 || f != math.Trunc(f) || f > math.MaxInt32 {
		return 0, fmt.Errorf("change set: invalid length %s", raw)
	}
	return int(f), nil
}

// lenBefore is the length of the document the change set applies to.
func (c changes) lenBefore() int {
	n := 0
	for _, p := range c {
		n += p.n
	}
	return n
}

// apply returns the changed document; doc is not modified.
func (c changes) apply(doc []uint16) ([]uint16, error) {
	if c.lenBefore() != len(doc) {
		return nil, fmt.Errorf("change set for a document of length %d, not %d", c.lenBefore(), len(doc))
	}
	size := len(doc)
	for _, p := range c {
		if p.replace {
			size += len(p.ins) - p.n
		}
	}
	out := make([]uint16, 0, size)
	pos := 0
	for _, p := range c {
		if p.replace {
			out = append(out, p.ins...)
		} else {
			out = append(out, doc[pos:pos+p.n]...)
		}
		pos += p.n
	}
	return out, nil
}

// replaceJSON describes, as a ChangeSet in JSON, the change from a to b: the
// part between their common prefix and suffix is replaced.
func replaceJSON(a, b []uint16) json.RawMessage {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	var parts []any
	if pre > 0 {
		parts = append(parts, pre)
	}
	del := len(a) - pre - suf
	ins := b[pre : len(b)-suf]
	if del > 0 || len(ins) > 0 {
		r := []any{del}
		if len(ins) > 0 {
			for _, line := range strings.Split(toString(ins), "\n") {
				r = append(r, line)
			}
		}
		parts = append(parts, r)
	}
	if suf > 0 {
		parts = append(parts, suf)
	}
	if parts == nil {
		parts = []any{}
	}
	out, _ := json.Marshal(parts)
	return out
}

// diffJSON describes the change from a to b as a ChangeSet in JSON that
// touches only the lines that differ, so cursors elsewhere stay where they
// are (merges of changes made on disk).
func diffJSON(a, b string) json.RawMessage {
	o, x := splitLines(a), splitLines(b)
	var parts []any
	keep := 0
	units := func(lines []string) int {
		n := 0
		for _, l := range lines {
			n += len(utf16.Encode([]rune(l)))
		}
		return n
	}
	pos := 0
	for _, h := range diffLines(o, x) {
		keep += units(o[pos:h.o0])
		if keep > 0 {
			parts = append(parts, keep)
			keep = 0
		}
		r := []any{units(o[h.o0:h.o1])}
		if ins := strings.Join(x[h.x0:h.x1], ""); ins != "" {
			for _, line := range strings.Split(ins, "\n") {
				r = append(r, line)
			}
		}
		parts = append(parts, r)
		pos = h.o1
	}
	if rest := units(o[pos:]); rest > 0 {
		parts = append(parts, rest)
	}
	if parts == nil {
		parts = []any{}
	}
	out, _ := json.Marshal(parts)
	return out
}

// fromString turns text into a document (line breaks already normalized).
func fromString(s string) []uint16 { return utf16.Encode([]rune(s)) }

// toString turns a document back into text. A lone surrogate (an edit that
// split a character, which editors do not produce) becomes U+FFFD.
func toString(d []uint16) string { return string(utf16.Decode(d)) }

// utf8Len is the size of the document as UTF-8 (without decoding it).
func utf8Len(d []uint16) int64 {
	var n int64
	for i := 0; i < len(d); i++ {
		switch c := d[i]; {
		case c < 0x80:
			n++
		case c < 0x800:
			n += 2
		case utf16.IsSurrogate(rune(c)) && c < 0xdc00 && i+1 < len(d) && d[i+1] >= 0xdc00 && d[i+1] < 0xe000:
			n += 4 // a pair
			i++
		default:
			n += 3 // includes a lone surrogate, written as U+FFFD
		}
	}
	return n
}
