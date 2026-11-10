package server

import (
	"context"
	"net/http"
	"strconv"

	"dendronotes/internal/drawing"
	"dendronotes/internal/httpx"
	"dendronotes/internal/index"
	"dendronotes/internal/mdparse"
	"dendronotes/internal/soma"
	"dendronotes/internal/somas"
)

const (
	defaultSearchLimit = 50
	maxSearchLimit     = 200
	maxQueryBytes      = 512
)

// handleSearch runs a full-text search. Words match as prefixes, "quoted
// text" as a phrase, #tag filters by tag (nested tags included).
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	q := r.URL.Query().Get("q")
	if len(q) > maxQueryBytes {
		httpx.Error(w, http.StatusBadRequest, "query_too_long")
		return
	}
	limit := defaultSearchLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxSearchLimit {
			httpx.Error(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = n
	}
	hits, err := rt.Index.Search(r.Context(), q, limit)
	if err != nil {
		s.internalError(w, r, "search", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"hits": hits, "complete": rt.Index.Ready()})
}

// handleBacklinks lists the notes with synapses to a note ("Incoming synapses").
func (s *Server) handleBacklinks(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	p, err := soma.CleanPath(r.PathValue("path"))
	if err != nil {
		s.somaError(w, r, err)
		return
	}
	bl, err := rt.Index.Backlinks(r.Context(), p)
	if err != nil {
		s.internalError(w, r, "backlinks", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"backlinks": bl})
}

// handleTags lists all tags with the number of notes using them.
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	tags, err := rt.Index.Tags(r.Context())
	if err != nil {
		s.internalError(w, r, "tags", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

// handleGraph returns all notes and the synapses between them.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	g, err := rt.Index.Graph(r.Context())
	if err == nil {
		err = addDrawings(r.Context(), rt, &g)
	}
	if err != nil {
		s.internalError(w, r, "graph", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, g)
}

// addDrawings puts the soma's drawings into the graph, each embedded (or
// linked) drawing connected to the notes that do so – resolved like every
// attachment reference.
func addDrawings(ctx context.Context, rt *somas.Runtime, g *index.Graph) error {
	entries, err := rt.Soma.Tree()
	if err != nil {
		return err
	}
	var files []string
	isDrawing := map[string]bool{}
	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		files = append(files, e.Path)
		if drawing.IsDrawing(e.Path) {
			isDrawing[e.Path] = true
			g.Nodes = append(g.Nodes, index.GraphNode{ID: e.Path, Kind: "drawing"})
		}
	}
	if len(isDrawing) == 0 {
		return nil
	}
	notes := map[string]bool{}
	for _, n := range g.Nodes {
		if n.Kind == "" && !n.Missing {
			notes[n.ID] = true
		}
	}
	refs, err := rt.Index.FileRefs(ctx)
	if err != nil {
		return err
	}
	res := mdparse.NewFileResolver(files)
	seen := map[index.GraphEdge]bool{}
	for _, ref := range refs {
		to := res.Resolve(ref.Target, ref.Markdown, ref.Note)
		e := index.GraphEdge{Source: ref.Note, Target: to}
		if !notes[ref.Note] || !isDrawing[to] || seen[e] {
			continue
		}
		seen[e] = true
		g.Embeds = append(g.Embeds, e)
	}
	return nil
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, what string, err error) {
	if r.Context().Err() == nil {
		s.log.Error(what+" failed", "err", err)
	}
	httpx.Error(w, http.StatusInternalServerError, "internal_error")
}
