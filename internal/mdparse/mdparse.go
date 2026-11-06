// Package mdparse extracts what the index needs from a note: synapses and tags.
//
// It mirrors the frontend (web/src/lib/synapses.ts) rule for rule, so that the
// synapses a note shows and the backlinks other notes list always agree. A
// shared fixture (web/src/lib/testdata/synapses.json) is checked by the tests
// of both sides.
package mdparse

import (
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"dendronotes/internal/soma"
)

// Synapse is a connection from a note to another note, as written in the source.
type Synapse struct {
	// Target is the note name or path as written, without #heading and |alias.
	Target  string `json:"target"`
	Heading string `json:"heading,omitempty"`
	Alias   string `json:"alias,omitempty"`
	// Embed is ![[Note]] instead of [[Note]].
	Embed bool `json:"embed"`
	// Markdown is a Markdown link [Text](path.md) instead of a wiki link.
	Markdown bool `json:"markdown"`
	// Start and End are byte offsets of the whole construct; Line is 0-based.
	Start int `json:"-"`
	End   int `json:"-"`
	Line  int `json:"line"`
}

// Doc is the parsed content of a note.
type Doc struct {
	Synapses []Synapse
	Tags     []string // without "#", in order of first appearance, deduplicated case-insensitively
}

// Parse extracts synapses and tags from a note.
func Parse(src string) Doc {
	masked := MaskCode(src)
	return Doc{Synapses: parseSynapses(src, masked), Tags: parseTags(src, masked)}
}

// attachmentExtensions are file types that are attachments, never notes.
var attachmentExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".avif": true,
	".svg": true, ".bmp": true, ".ico": true, ".pdf": true, ".mp3": true, ".wav": true,
	".ogg": true, ".m4a": true, ".flac": true, ".mp4": true, ".webm": true, ".mov": true,
	".mkv": true, ".zip": true, ".csv": true, ".json": true, ".html": true, ".css": true,
	".js": true,
}

// IsNoteTarget reports whether a wiki link target names a note (not an attachment).
func IsNoteTarget(target string) bool {
	return !attachmentExtensions[strings.ToLower(path.Ext(target))]
}

var (
	fenceOpen  = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	fenceClose = regexp.MustCompile("^ {0,3}[`~]+\\s*$")
)

// MaskCode replaces fenced code blocks and inline code spans with spaces,
// keeping every byte offset, so that brackets inside code are never read as
// synapses.
func MaskCode(src string) string {
	lines := strings.Split(src, "\n")
	var fenceChar byte
	fenceLen := 0
	for i, line := range lines {
		m := fenceOpen.FindStringSubmatchIndex(line)
		if fenceLen > 0 {
			if m != nil && line[m[2]] == fenceChar && m[3]-m[2] >= fenceLen && fenceClose.MatchString(line) {
				fenceLen = 0
			}
			lines[i] = blank(line)
			continue
		}
		if m != nil && !(line[m[2]] == '`' && strings.Contains(line[m[1]:], "`")) {
			fenceChar, fenceLen = line[m[2]], m[3]-m[2]
			lines[i] = blank(line)
			continue
		}
		lines[i] = maskInlineCode(line)
	}
	return strings.Join(lines, "\n")
}

// blank returns as many spaces as s has bytes.
func blank(s string) string { return strings.Repeat(" ", len(s)) }

// maskInlineCode blanks code spans the way the frontend's regular expression
// /(`+)(.+?)\1(?!`)/g does, including its backtracking: an opening run of n
// backticks may also match with fewer of them.
func maskInlineCode(line string) string {
	if !strings.Contains(line, "`") {
		return line
	}
	b := []byte(line)
	for i := 0; i < len(b); {
		if b[i] != '`' {
			i++
			continue
		}
		run := 0
		for i+run < len(b) && b[i+run] == '`' {
			run++
		}
		end := -1
		for n := run; n >= 1 && end < 0; n-- {
			end = findCloser(b, i+n+1, n)
		}
		if end < 0 {
			i++
			continue
		}
		for j := i; j < end; j++ {
			b[j] = ' '
		}
		i = end
	}
	return string(b)
}

// findCloser returns the end of the first run of exactly n backticks
// (not followed by another backtick) starting at or after from, or -1.
func findCloser(b []byte, from, n int) int {
	for k := from; k+n <= len(b); k++ {
		ok := true
		for j := 0; j < n; j++ {
			if b[k+j] != '`' {
				ok = false
				break
			}
		}
		if ok && (k+n == len(b) || b[k+n] != '`') {
			return k + n
		}
	}
	return -1
}

var (
	wikiLink = regexp.MustCompile(`(!?)\[\[([^\[\]\n]+?)\]\]`)
	mdLink   = regexp.MustCompile(`(!?)\[([^\]\n]*)\]\(\s*(<[^<>\n]+>|[^)\s]+)(?:\s+"[^"\n]*")?\s*\)`)
	scheme   = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*:`)
)

// SplitWikiTarget splits the inside of [[…]] into target, heading and alias.
func SplitWikiTarget(inner string) (target, heading, alias string) {
	rest := inner
	if i := strings.IndexByte(rest, '|'); i >= 0 {
		alias = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		heading = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	return strings.TrimSpace(rest), heading, alias
}

// parseSynapses reads synapses from the masked text (like the frontend), so
// code inside a construct is blank there too; offsets are the same in both.
func parseSynapses(src, masked string) []Synapse {
	var out []Synapse
	for _, m := range wikiLink.FindAllStringSubmatchIndex(masked, -1) {
		target, heading, alias := SplitWikiTarget(masked[m[4]:m[5]])
		if target == "" || !IsNoteTarget(target) {
			continue // heading in the same note, or an attachment
		}
		out = append(out, Synapse{
			Target: target, Heading: heading, Alias: alias,
			Embed: m[3] > m[2], Start: m[0], End: m[1],
		})
	}
	for _, m := range mdLink.FindAllStringSubmatchIndex(masked, -1) {
		href := masked[m[6]:m[7]]
		if strings.HasPrefix(href, "<") {
			if len(href) < 2 || !strings.HasSuffix(href, ">") {
				continue // unclosed <…: not a link destination
			}
			href = href[1 : len(href)-1]
		}
		if scheme.MatchString(href) || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "//") {
			continue
		}
		p, heading := href, ""
		if i := strings.IndexByte(href, '#'); i >= 0 {
			p, heading = href[:i], decodeSafe(href[i+1:])
		}
		p = decodeSafe(p)
		if p == "" || !soma.IsNote(p) {
			continue
		}
		out = append(out, Synapse{
			Target: p, Heading: heading, Alias: masked[m[4]:m[5]],
			Embed: m[3] > m[2], Markdown: true, Start: m[0], End: m[1],
		})
	}
	slices.SortStableFunc(out, func(a, b Synapse) int { return a.Start - b.Start })
	for i := range out {
		out[i].Line = strings.Count(src[:out[i].Start], "\n")
	}
	return out
}

// decodeSafe percent-decodes like JavaScript's decodeURIComponent and returns
// s unchanged where that would throw.
func decodeSafe(s string) string {
	d, err := url.PathUnescape(s)
	if err != nil || !utf8.ValidString(d) {
		return s
	}
	return d
}

// inlineTag matches #tag after whitespace or at the start of a line. Tags
// consist of letters, digits, "_", "-" and "/" (nested tags).
var inlineTag = regexp.MustCompile(`(?:^|\s)#([\p{L}\p{N}_/-]+)`)

// parseTags collects inline #tags outside code and the tags of the front matter.
func parseTags(src, masked string) []string {
	var tags []string
	seen := map[string]bool{}
	add := func(t string) {
		t = strings.Trim(strings.TrimPrefix(strings.TrimSpace(t), "#"), "/")
		if t == "" || !hasNonDigit(t) {
			return
		}
		if k := Fold(t); !seen[k] {
			seen[k] = true
			tags = append(tags, t)
		}
	}
	fm, bodyStart := FrontMatter(src)
	for _, t := range frontMatterList(fm, "tags", "tag") {
		add(t)
	}
	for _, m := range inlineTag.FindAllStringSubmatchIndex(masked[bodyStart:], -1) {
		add(masked[bodyStart+m[2] : bodyStart+m[3]])
	}
	return tags
}

func hasNonDigit(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// FrontMatter returns the YAML front matter of a note (without the "---"
// delimiters) and the byte offset where the body starts. Notes without front
// matter return "" and 0.
func FrontMatter(src string) (string, int) {
	first, rest, ok := strings.Cut(src, "\n")
	if !ok || strings.TrimRight(first, " \t\r") != "---" {
		return "", 0
	}
	offset := len(first) + 1
	for pos := 0; pos <= len(rest); {
		line, _, found := strings.Cut(rest[pos:], "\n")
		if t := strings.TrimRight(line, " \t\r"); t == "---" || t == "..." {
			end := offset + pos + len(line)
			if found {
				end++
			}
			return rest[:pos], end
		}
		if !found {
			break
		}
		pos += len(line) + 1
	}
	return "", 0
}

// frontMatterList reads a list value from simple YAML front matter: inline
// (`tags: [a, b]`, `tags: a, b`) or as a block of "- item" lines. It covers
// what note front matter uses in practice, not YAML in general.
func frontMatterList(fm string, keys ...string) []string {
	lines := strings.Split(fm, "\n")
	for i := 0; i < len(lines); i++ {
		key, value, ok := strings.Cut(strings.TrimRight(lines[i], "\r"), ":")
		if !ok || strings.HasPrefix(key, " ") || !slices.Contains(keys, strings.ToLower(strings.TrimSpace(key))) {
			continue
		}
		value = strings.TrimSpace(value)
		var items []string
		if value != "" {
			value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
			sep := ","
			if !strings.Contains(value, ",") {
				sep = " "
			}
			for _, it := range strings.Split(value, sep) {
				items = append(items, unquote(it))
			}
			return items
		}
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimSpace(strings.TrimRight(lines[j], "\r"))
			if !strings.HasPrefix(t, "-") {
				break
			}
			items = append(items, unquote(strings.TrimPrefix(t, "-")))
		}
		return items
	}
	return nil
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	}
	return s
}

// LinkDestination returns the byte range of the destination in a Markdown
// link such as [text](dest "title") (including <…> brackets, if any).
func LinkDestination(link string) (start, end int, ok bool) {
	m := mdLink.FindStringSubmatchIndex(link)
	if m == nil || m[0] != 0 {
		return 0, 0, false
	}
	return m[6], m[7], true
}
