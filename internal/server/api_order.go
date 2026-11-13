package server

import (
	"net/http"
	"slices"
	"strings"

	"dendronotes/internal/httpx"
	"dendronotes/internal/index"
	"dendronotes/internal/soma"
	"dendronotes/internal/store"
)

// handleSetOrder sets how a folder's entries are sorted for everyone with
// access: by name, by date, or in the order of names (manual).
func (s *Server) handleSetOrder(w http.ResponseWriter, r *http.Request) {
	rt := runtimeOf(r)
	var body struct {
		Folder string   `json:"folder"`
		Mode   string   `json:"mode"`
		Names  []string `json:"names"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1<<20); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	folder := body.Folder
	if folder != "" {
		clean, err := soma.CleanPath(folder)
		if err != nil {
			s.somaError(w, r, err)
			return
		}
		e, err := rt.Soma.Stat(clean)
		if err != nil {
			s.somaError(w, r, err)
			return
		}
		if e.Type != "dir" {
			httpx.Error(w, http.StatusBadRequest, "not_a_folder")
			return
		}
		folder = clean
	}
	if !slices.Contains(store.SortModes, body.Mode) {
		httpx.Error(w, http.StatusBadRequest, "invalid_mode")
		return
	}
	if len(body.Names) > store.MaxOrderNames {
		httpx.Error(w, http.StatusBadRequest, "too_many_names")
		return
	}
	for _, n := range body.Names {
		if n == "" || len(n) > 255 || strings.ContainsAny(n, "/\\\x00") || n == "." || n == ".." {
			httpx.Error(w, http.StatusBadRequest, "invalid_name")
			return
		}
	}
	if err := s.store.SetFolderOrder(r.Context(), rt.ID, folder, store.FolderOrder{Mode: body.Mode, Names: body.Names}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal")
		return
	}
	// Open trees load again and show the new order.
	rt.Publish(index.Changes{Tree: true})
	w.WriteHeader(http.StatusNoContent)
}
