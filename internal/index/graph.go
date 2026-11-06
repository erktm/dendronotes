package index

import (
	"context"
	"slices"
	"strings"

	"dendronotes/internal/mdparse"
)

// MissingPrefix starts the id of a node for a note that does not exist yet
// (the target of unresolved synapses); the rest is its folded name.
const MissingPrefix = "?"

// GraphNode is a note, or a missing note that synapses point to.
type GraphNode struct {
	ID      string   `json:"id"`              // soma path, or MissingPrefix + name
	Label   string   `json:"label,omitempty"` // only for missing notes: the name as written
	Missing bool     `json:"missing,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// GraphEdge is a synapse between two nodes (at most one per direction).
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Graph is the network of notes and synapses.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Graph returns all notes with the synapses between them, resolved with the
// same rules as everywhere else. Synapses of a note to itself are left out.
func (ix *Index) Graph(ctx context.Context) (Graph, error) {
	res := ix.Resolver()
	notes := slices.Clone(res.Notes())
	slices.Sort(notes)
	known := make(map[string]int, len(notes))
	g := Graph{Nodes: make([]GraphNode, 0, len(notes)), Edges: []GraphEdge{}}
	for i, p := range notes {
		known[p] = i
		g.Nodes = append(g.Nodes, GraphNode{ID: p})
	}

	tags, err := ix.db.QueryContext(ctx, `SELECT n.path, t.tag FROM tags t JOIN notes n ON n.id = t.note_id ORDER BY t.rowid`)
	if err != nil {
		return Graph{}, err
	}
	for tags.Next() {
		var p, t string
		if err := tags.Scan(&p, &t); err != nil {
			tags.Close()
			return Graph{}, err
		}
		if i, ok := known[p]; ok {
			g.Nodes[i].Tags = append(g.Nodes[i].Tags, t)
		}
	}
	tags.Close()
	if err := tags.Err(); err != nil {
		return Graph{}, err
	}

	rows, err := ix.db.QueryContext(ctx, `SELECT n.path, s.target, s.markdown
		FROM synapses s JOIN notes n ON n.id = s.note_id ORDER BY n.path, s.seq`)
	if err != nil {
		return Graph{}, err
	}
	defer rows.Close()
	seen := map[GraphEdge]bool{}
	missing := map[string]bool{}
	for rows.Next() {
		var (
			src, target string
			markdown    bool
		)
		if err := rows.Scan(&src, &target, &markdown); err != nil {
			return Graph{}, err
		}
		if _, ok := known[src]; !ok {
			continue // indexed a moment before the resolver knows it
		}
		to := res.Resolve(target, markdown, src)
		if to == "" {
			name := missingName(target)
			to = MissingPrefix + mdparse.Fold(name)
			if !missing[to] {
				missing[to] = true
				g.Nodes = append(g.Nodes, GraphNode{ID: to, Label: name, Missing: true})
			}
		}
		e := GraphEdge{Source: src, Target: to}
		if to == src || seen[e] {
			continue
		}
		seen[e] = true
		g.Edges = append(g.Edges, e)
	}
	return g, rows.Err()
}

// missingName is the display name of a missing note: its file name without ".md".
func missingName(target string) string {
	name := target[strings.LastIndex(target, "/")+1:]
	return strings.TrimSuffix(name, ".md")
}
