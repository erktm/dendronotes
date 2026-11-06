// Package relink rewrites synapses when notes are renamed or moved, so that
// every synapse that pointed to a note still points to it afterwards.
//
// Planning happens before the move (with the old paths), writing after it;
// each edit carries the version it was computed from, so a note changed in
// between is left alone rather than overwritten.
package relink

import (
	"path"
	"slices"
	"strings"

	"dendronotes/internal/mdparse"
	"dendronotes/internal/soma"
)

// Edit is the new content for one note.
type Edit struct {
	Path     string // where the note lives after the move
	ETag     string // version the edit is based on
	Content  string
	Synapses int // number of rewritten synapses
}

// ReadFunc reads a note before the move.
type ReadFunc func(p string) (content, etag string, err error)

// Plan computes the edits for moving notes according to moves (old path →
// new path). sources are the notes that may refer to moved notes; moved notes
// are always checked too, since their relative links change with them.
// notes lists all notes before the move.
func Plan(read ReadFunc, sources, notes []string, moves map[string]string) ([]Edit, error) {
	oldRes := mdparse.NewResolver(notes)
	newNotes := make([]string, len(notes))
	for i, n := range notes {
		newNotes[i] = moved(moves, n)
	}
	newRes := mdparse.NewResolver(newNotes)

	check := slices.Clone(sources)
	for from := range moves {
		check = append(check, from)
	}
	slices.Sort(check)
	check = slices.Compact(check)

	var edits []Edit
	for _, src := range check {
		content, etag, err := read(src)
		if err != nil {
			continue // unreadable or too large: nothing to rewrite
		}
		newSrc := moved(moves, src)
		out, n := rewrite(content, src, newSrc, moves, oldRes, newRes)
		if n > 0 {
			edits = append(edits, Edit{Path: newSrc, ETag: etag, Content: out, Synapses: n})
		}
	}
	return edits, nil
}

func moved(moves map[string]string, p string) string {
	if to, ok := moves[p]; ok {
		return to
	}
	return p
}

// rewrite returns content with every synapse fixed that would no longer
// reach its note after the move, and the number of changes.
func rewrite(content, src, newSrc string, moves map[string]string, oldRes, newRes *mdparse.Resolver) (string, int) {
	syns := mdparse.Parse(content).Synapses
	var b strings.Builder
	last, count := 0, 0
	for _, s := range syns {
		target := oldRes.Resolve(s.Target, s.Markdown, src)
		if target == "" {
			continue // unresolved before: leave it
		}
		newTarget := moved(moves, target)
		if newRes.Resolve(s.Target, s.Markdown, newSrc) == newTarget {
			continue // still correct, e.g. [[Name]] after moving to another folder
		}
		var repl string
		if s.Markdown {
			repl = markdownLink(content[s.Start:s.End], newSrc, newTarget, newRes)
		} else {
			repl = wikiLink(s, newSrc, newTarget, newRes)
		}
		if repl == "" {
			continue
		}
		b.WriteString(content[last:s.Start])
		b.WriteString(repl)
		last = s.End
		count++
	}
	if count == 0 {
		return content, 0
	}
	b.WriteString(content[last:])
	return b.String(), count
}

// wikiLink writes [[target#heading|alias]] for the new location: the bare
// name when that is unambiguous (and was used before), the path otherwise.
func wikiLink(s mdparse.Synapse, newSrc, newTarget string, res *mdparse.Resolver) string {
	keepExt := soma.IsNote(s.Target)
	strip := func(p string) string {
		if keepExt {
			return p
		}
		return strings.TrimSuffix(p, path.Ext(p))
	}
	target := strip(newTarget)
	if name := strip(path.Base(newTarget)); !strings.Contains(s.Target, "/") && res.Resolve(name, false, newSrc) == newTarget {
		target = name
	}
	if res.Resolve(target, false, newSrc) != newTarget {
		return ""
	}
	var b strings.Builder
	if s.Embed {
		b.WriteString("!")
	}
	b.WriteString("[[")
	b.WriteString(target)
	if s.Heading != "" {
		b.WriteString("#" + s.Heading)
	}
	if s.Alias != "" {
		b.WriteString("|" + s.Alias)
	}
	b.WriteString("]]")
	return b.String()
}

// markdownLink replaces the destination of [text](dest "title") with the
// relative path from the note's new folder, keeping text, title and #heading.
func markdownLink(link, newSrc, newTarget string, res *mdparse.Resolver) string {
	start, end, ok := mdparse.LinkDestination(link)
	if !ok {
		return ""
	}
	dest := link[start:end]
	angle := strings.HasPrefix(dest, "<")
	if angle {
		dest = dest[1 : len(dest)-1]
	}
	fragment := ""
	if i := strings.IndexByte(dest, '#'); i >= 0 {
		fragment = dest[i:]
	}
	rel := relPath(mdparse.Dir(newSrc), newTarget)
	if res.Resolve(rel, true, newSrc) != newTarget {
		return ""
	}
	var newDest string
	if angle {
		newDest = "<" + rel + fragment + ">"
	} else {
		newDest = escapeDest(rel) + fragment
	}
	return link[:start] + newDest + link[end:]
}

// relPath is the path to target relative to the folder dir.
func relPath(dir, target string) string {
	var from []string
	if dir != "" {
		from = strings.Split(dir, "/")
	}
	to := strings.Split(target, "/")
	i := 0
	for i < len(from) && i < len(to)-1 && from[i] == to[i] {
		i++
	}
	parts := make([]string, 0, len(from)-i+len(to)-i)
	for range from[i:] {
		parts = append(parts, "..")
	}
	return strings.Join(append(parts, to[i:]...), "/")
}

// escapeDest percent-encodes what a bare link destination cannot contain.
var destEscaper = strings.NewReplacer("%", "%25", " ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E", "#", "%23", "?", "%3F")

func escapeDest(p string) string { return destEscaper.Replace(p) }
