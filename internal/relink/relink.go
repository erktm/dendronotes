// Package relink rewrites synapses when notes are renamed or moved, and
// references to attachments (embeds and links) when files are, so that every
// one still points where it pointed before.
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
	Embeds   int // number of rewritten references to attachments
}

// ReadFunc reads a note before the move.
type ReadFunc func(p string) (content, etag string, err error)

// resolvers before and after the move, for notes and for all files.
type resolvers struct {
	oldNotes, newNotes, oldFiles, newFiles *mdparse.Resolver
}

// Plan computes the edits for moving files according to moves (old path →
// new path; notes and attachments). sources are the notes that may refer to
// moved files; moved notes are always checked too, since their relative
// links change with them. notes lists all notes, files all files (notes
// included) before the move.
func Plan(read ReadFunc, sources, notes, files []string, moves map[string]string) ([]Edit, error) {
	after := func(list []string) []string {
		out := make([]string, len(list))
		for i, p := range list {
			out[i] = moved(moves, p)
		}
		return out
	}
	r := resolvers{
		oldNotes: mdparse.NewResolver(notes), newNotes: mdparse.NewResolver(after(notes)),
		oldFiles: mdparse.NewFileResolver(files), newFiles: mdparse.NewFileResolver(after(files)),
	}

	check := slices.Clone(sources)
	for from := range moves {
		if soma.IsNote(from) {
			check = append(check, from)
		}
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
		out, syn, emb := rewrite(content, src, newSrc, moves, r)
		if syn+emb > 0 {
			edits = append(edits, Edit{Path: newSrc, ETag: etag, Content: out, Synapses: syn, Embeds: emb})
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

// ref is a synapse or a reference to an attachment.
type ref struct {
	mdparse.Synapse
	file bool
}

// rewrite returns content with every synapse and attachment reference fixed
// that would no longer reach its target after the move, and the numbers of
// changes.
func rewrite(content, src, newSrc string, moves map[string]string, r resolvers) (string, int, int) {
	doc := mdparse.Parse(content)
	refs := make([]ref, 0, len(doc.Synapses)+len(doc.Files))
	for _, s := range doc.Synapses {
		refs = append(refs, ref{s, false})
	}
	for _, f := range doc.Files {
		refs = append(refs, ref{f, true})
	}
	slices.SortFunc(refs, func(a, b ref) int { return a.Start - b.Start })

	var b strings.Builder
	last, syn, emb := 0, 0, 0
	for _, s := range refs {
		oldRes, newRes := r.oldNotes, r.newNotes
		if s.file {
			oldRes, newRes = r.oldFiles, r.newFiles
		}
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
			repl = wikiLink(s.Synapse, newSrc, newTarget, newRes, s.file)
		}
		if repl == "" {
			continue
		}
		b.WriteString(content[last:s.Start])
		b.WriteString(repl)
		last = s.End
		if s.file {
			emb++
		} else {
			syn++
		}
	}
	if syn+emb == 0 {
		return content, 0, 0
	}
	b.WriteString(content[last:])
	return b.String(), syn, emb
}

// wikiLink writes [[target#heading|alias]] for the new location: the bare
// name when that is unambiguous (and was used before), the path otherwise.
// Attachments always keep their extension.
func wikiLink(s mdparse.Synapse, newSrc, newTarget string, res *mdparse.Resolver, file bool) string {
	keepExt := file || soma.IsNote(s.Target)
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
