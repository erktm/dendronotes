package mdparse

import (
	"path"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"dendronotes/internal/soma"
)

// Fold is the case-insensitive comparison key for names and paths (NFC,
// lowercase), matching the frontend.
func Fold(s string) string { return strings.ToLower(norm.NFC.String(s)) }

// NormalizePath resolves "." and ".." segments. ok is false if the path
// would leave the soma.
func NormalizePath(p string) (string, bool) {
	var out []string
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "", ".":
		case "..":
			if len(out) == 0 {
				return "", false
			}
			out = out[:len(out)-1]
		default:
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/"), true
}

// WithNoteExt adds ".md" unless the name already has a note extension.
func WithNoteExt(t string) string {
	if soma.IsNote(t) {
		return t
	}
	return t + ".md"
}

// Dir is the folder of a soma path ("" for the top level).
func Dir(p string) string {
	if d := path.Dir(p); d != "." {
		return d
	}
	return ""
}

// NameKey is the lookup key of a synapse target: the folded file name with
// note extension. A note can only be the target of synapses whose key equals
// NameKey(its path).
func NameKey(target string) string {
	return Fold(path.Base(WithNoteExt(strings.TrimLeft(target, "/"))))
}

// Resolver finds the note a synapse points to within a fixed set of notes.
type Resolver struct {
	notes  []string
	set    map[string]bool
	byFold map[string]string
	byName map[string][]string // folded file name → notes
}

// NewResolver indexes the soma-relative paths of all notes.
func NewResolver(notes []string) *Resolver {
	r := &Resolver{
		notes:  notes,
		set:    make(map[string]bool, len(notes)),
		byFold: make(map[string]string, len(notes)),
		byName: make(map[string][]string, len(notes)),
	}
	for _, n := range notes {
		r.set[n] = true
		r.byFold[Fold(n)] = n
		k := Fold(path.Base(n))
		r.byName[k] = append(r.byName[k], n)
	}
	return r
}

// Notes returns the paths the resolver was built from.
func (r *Resolver) Notes() []string { return r.notes }

func (r *Resolver) exact(p string, ok bool) string {
	if !ok {
		return ""
	}
	if r.set[p] {
		return p
	}
	return r.byFold[Fold(p)]
}

// Resolve returns the note a synapse in note from points to, or "" if it is
// unresolved: a Markdown link is relative to the current note; a wiki link
// matches by path or by file name (case-insensitive). Several candidates:
// same folder first, then the shortest path.
func (r *Resolver) Resolve(target string, markdown bool, from string) string {
	here := Dir(from)
	if markdown {
		if p := r.exact(NormalizePath(here + "/" + target)); p != "" {
			return p
		}
		return r.exact(NormalizePath(target))
	}

	t := WithNoteExt(strings.TrimLeft(target, "/"))
	var candidates []string
	if strings.Contains(t, "/") {
		if p := r.exact(NormalizePath(t)); p != "" {
			return p
		}
		if p := r.exact(NormalizePath(here + "/" + t)); p != "" {
			return p
		}
		suffix := "/" + Fold(t)
		for _, n := range r.byName[Fold(path.Base(t))] {
			if strings.HasSuffix(Fold(n), suffix) {
				candidates = append(candidates, n)
			}
		}
	} else {
		candidates = r.byName[Fold(t)]
	}
	switch len(candidates) {
	case 0:
		return ""
	case 1:
		return candidates[0]
	}
	return slices.MinFunc(candidates, func(a, b string) int {
		if sa, sb := Dir(a) == here, Dir(b) == here; sa != sb {
			if sa {
				return -1
			}
			return 1
		}
		if da, db := strings.Count(a, "/"), strings.Count(b, "/"); da != db {
			return da - db
		}
		return strings.Compare(a, b)
	})
}
