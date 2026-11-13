package server

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"dendronotes/internal/auth"
	"dendronotes/internal/drawing"
	"dendronotes/internal/httpx"
	"dendronotes/internal/store"
)

// Icon libraries: pictures to put into drawings. Every soma has one (its
// members use it, contributors add, the uploader or an editor deletes), and
// the server has one that admins fill for everyone. Uploads are PNG (the
// browser converts other pictures and scales them down) or checked SVG,
// single or as a ZIP pack whose folders become sections. Drawings embed their
// own copy, so deleting an icon here changes no drawing.

// iconLibrary is a library and what the caller may do in it.
type iconLibrary struct {
	id string
	// mayDelete decides about icons and sections of others.
	mayDelete func(r *http.Request, createdBy int64) bool
}

func (s *Server) somaLibrary(r *http.Request) iconLibrary {
	return iconLibrary{id: runtimeOf(r).ID, mayDelete: func(r *http.Request, createdBy int64) bool {
		me, _ := auth.UserFrom(r.Context())
		_, role := somaOf(r)
		return createdBy == me.ID || store.RoleRank(role) >= store.RoleRank(store.MemberEditor)
	}}
}

var serverLibrary = iconLibrary{id: store.ServerIcons, mayDelete: func(*http.Request, int64) bool { return true }}

// pathID reads {id}.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return 0, false
	}
	return id, true
}

// iconLabel checks the name of an icon or a section.
func iconLabel(w http.ResponseWriter, name, fallback string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	if name == "" || utf8.RuneCountInString(name) > 80 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_name")
		return "", false
	}
	return name, true
}

// libraryContent is what a library holds.
type libraryContent struct {
	Icons    []store.Icon        `json:"icons"`
	Sections []store.IconSection `json:"sections"`
}

func (s *Server) libraryContent(r *http.Request, library string) (libraryContent, error) {
	icons, err := s.store.Icons(r.Context(), library)
	if err != nil {
		return libraryContent{}, err
	}
	sections, err := s.store.IconSections(r.Context(), library)
	return libraryContent{Icons: icons, Sections: sections}, err
}

// handleIcons lists the soma's library and the server's.
func (s *Server) handleIcons(w http.ResponseWriter, r *http.Request) {
	own, err := s.libraryContent(r, runtimeOf(r).ID)
	if err != nil {
		s.internalError(w, r, "listing icons", err)
		return
	}
	shared, err := s.libraryContent(r, store.ServerIcons)
	if err != nil {
		s.internalError(w, r, "listing icons", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"icons": own.Icons, "sections": own.Sections, "server": shared})
}

func (s *Server) handleAdminIcons(w http.ResponseWriter, r *http.Request) {
	c, err := s.libraryContent(r, store.ServerIcons)
	if err != nil {
		s.internalError(w, r, "listing icons", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

// serveIcon delivers an icon's picture (sandboxed like every file).
func (s *Server) serveIcon(w http.ResponseWriter, r *http.Request, library string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	icon, data, err := s.store.IconByID(r.Context(), library, id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "reading icon", err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", icon.Mime)
	h.Set("Content-Security-Policy", fileCSP)
	h.Set("Cache-Control", "private, max-age=86400") // an icon never changes
	_, _ = w.Write(data)
}

func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	s.serveIcon(w, r, runtimeOf(r).ID)
}

// handleServerIcon serves the server's icons to everyone signed in.
func (s *Server) handleServerIcon(w http.ResponseWriter, r *http.Request) {
	s.serveIcon(w, r, store.ServerIcons)
}

// sectionParam reads ?section= (0 or missing: none).
func sectionParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	v := r.URL.Query().Get("section")
	if v == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id < 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_section")
		return 0, false
	}
	return id, true
}

// addIcon stores the request body as an icon named ?name=, in ?section=.
func (s *Server) addIcon(w http.ResponseWriter, r *http.Request, lib iconLibrary) {
	name, ok := iconLabel(w, r.URL.Query().Get("name"), "Icon")
	if !ok {
		return
	}
	section, ok := sectionParam(w, r)
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, drawing.MaxIconBytes+1))
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	mime, data, err := drawing.NormalizeIcon(data)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_icon")
		return
	}
	me, _ := auth.UserFrom(r.Context())
	icon, err := s.store.AddIcon(r.Context(), lib.id, name, mime, data, me.ID, section)
	switch {
	case errors.Is(err, store.ErrTooManyIcons):
		httpx.Error(w, http.StatusConflict, "too_many_icons")
	case errors.Is(err, store.ErrNotFound):
		httpx.Error(w, http.StatusBadRequest, "invalid_section")
	case err != nil:
		s.internalError(w, r, "adding icon", err)
	default:
		httpx.WriteJSON(w, http.StatusCreated, icon)
	}
}

func (s *Server) handleAddIcon(w http.ResponseWriter, r *http.Request) {
	s.addIcon(w, r, s.somaLibrary(r))
}
func (s *Server) handleAdminAddIcon(w http.ResponseWriter, r *http.Request) {
	s.addIcon(w, r, serverLibrary)
}

// maxReportedSkips bounds the list of skipped files in an answer.
const maxReportedSkips = 50

// addIconPack unpacks a ZIP of icons (named ?name=) into sections.
func (s *Server) addIconPack(w http.ResponseWriter, r *http.Request, lib iconLibrary) {
	packName, ok := iconLabel(w, strings.TrimSuffix(path.Base(r.URL.Query().Get("name")), path.Ext(r.URL.Query().Get("name"))), "Icons")
	if !ok {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, drawing.MaxPackBytes+1))
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	icons, skipped, err := drawing.ReadPack(data, packName)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_pack")
		return
	}
	me, _ := auth.UserFrom(r.Context())
	sections := map[string]store.IconSection{}
	var names []string
	added := 0
	for _, icon := range icons {
		sec, ok := sections[icon.Section]
		if !ok {
			sec, err = s.store.IconSection(r.Context(), lib.id, icon.Section)
			if errors.Is(err, store.ErrTooManyIcons) {
				skipped = append(skipped, drawing.PackSkip{File: icon.Section + "/" + icon.Name, Reason: "too many sections"})
				continue
			}
			if err != nil {
				s.internalError(w, r, "adding icon section", err)
				return
			}
			sections[icon.Section] = sec
			names = append(names, sec.Name)
		}
		if _, err := s.store.AddIcon(r.Context(), lib.id, icon.Name, icon.Mime, icon.Data, me.ID, sec.ID); errors.Is(err, store.ErrTooManyIcons) {
			skipped = append(skipped, drawing.PackSkip{File: icon.Section + "/" + icon.Name, Reason: "the library is full"})
			continue
		} else if err != nil {
			s.internalError(w, r, "adding icon", err)
			return
		}
		added++
	}
	if lib.id == store.ServerIcons {
		s.audit(r, "icons.pack", packName, strconv.Itoa(added)+" icons")
	}
	if len(skipped) > maxReportedSkips {
		skipped = skipped[:maxReportedSkips]
	}
	if names == nil {
		names = []string{}
	}
	if skipped == nil {
		skipped = []drawing.PackSkip{}
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"added": added, "sections": names, "skipped": skipped})
}

func (s *Server) handleAddIconPack(w http.ResponseWriter, r *http.Request) {
	s.addIconPack(w, r, s.somaLibrary(r))
}
func (s *Server) handleAdminAddIconPack(w http.ResponseWriter, r *http.Request) {
	s.addIconPack(w, r, serverLibrary)
}

// moveIcon puts an icon into another section ({"sectionId": 0} for none).
func (s *Server) moveIcon(w http.ResponseWriter, r *http.Request, lib iconLibrary) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		SectionID int64 `json:"sectionId"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	err := s.store.MoveIcon(r.Context(), lib.id, id, body.SectionID)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "moving icon", err)
		return
	}
	icon, _, err := s.store.IconByID(r.Context(), lib.id, id)
	if err != nil {
		s.internalError(w, r, "reading icon", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, icon)
}

func (s *Server) handleMoveIcon(w http.ResponseWriter, r *http.Request) {
	s.moveIcon(w, r, s.somaLibrary(r))
}
func (s *Server) handleAdminMoveIcon(w http.ResponseWriter, r *http.Request) {
	s.moveIcon(w, r, serverLibrary)
}

// deleteIcon removes an icon: in a soma its uploader or an editor.
func (s *Server) deleteIcon(w http.ResponseWriter, r *http.Request, lib iconLibrary) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	icon, _, err := s.store.IconByID(r.Context(), lib.id, id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "reading icon", err)
		return
	}
	if !lib.mayDelete(r, icon.CreatedBy) {
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := s.store.DeleteIcon(r.Context(), lib.id, id); err != nil {
		s.internalError(w, r, "deleting icon", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"id": id})
}

func (s *Server) handleDeleteIcon(w http.ResponseWriter, r *http.Request) {
	s.deleteIcon(w, r, s.somaLibrary(r))
}
func (s *Server) handleAdminDeleteIcon(w http.ResponseWriter, r *http.Request) {
	s.deleteIcon(w, r, serverLibrary)
}

// addIconSection makes a section ({"name": …}) at the end of a library.
func (s *Server) addIconSection(w http.ResponseWriter, r *http.Request, lib iconLibrary) {
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	name, ok := iconLabel(w, body.Name, "")
	if !ok {
		return
	}
	sec, err := s.store.AddIconSection(r.Context(), lib.id, name)
	if errors.Is(err, store.ErrTooManyIcons) {
		httpx.Error(w, http.StatusConflict, "too_many_sections")
		return
	}
	if err != nil {
		s.internalError(w, r, "adding icon section", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sec)
}

func (s *Server) handleAddIconSection(w http.ResponseWriter, r *http.Request) {
	s.addIconSection(w, r, s.somaLibrary(r))
}
func (s *Server) handleAdminAddIconSection(w http.ResponseWriter, r *http.Request) {
	s.addIconSection(w, r, serverLibrary)
}

// renameIconSection renames a section ({"name": …}).
func (s *Server) renameIconSection(w http.ResponseWriter, r *http.Request, lib iconLibrary) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	name, ok := iconLabel(w, body.Name, "")
	if !ok {
		return
	}
	err := s.store.RenameIconSection(r.Context(), lib.id, id, name)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "renaming icon section", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRenameIconSection(w http.ResponseWriter, r *http.Request) {
	s.renameIconSection(w, r, s.somaLibrary(r))
}
func (s *Server) handleAdminRenameIconSection(w http.ResponseWriter, r *http.Request) {
	s.renameIconSection(w, r, serverLibrary)
}

// orderIconSections puts the sections in the order of {"ids": […]}.
func (s *Server) orderIconSections(w http.ResponseWriter, r *http.Request, lib iconLibrary) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 8192); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if len(body.IDs) > store.MaxIconSections {
		httpx.Error(w, http.StatusBadRequest, "invalid_order")
		return
	}
	if err := s.store.OrderIconSections(r.Context(), lib.id, body.IDs); err != nil {
		s.internalError(w, r, "ordering icon sections", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOrderIconSections(w http.ResponseWriter, r *http.Request) {
	s.orderIconSections(w, r, s.somaLibrary(r))
}
func (s *Server) handleAdminOrderIconSections(w http.ResponseWriter, r *http.Request) {
	s.orderIconSections(w, r, serverLibrary)
}

// deleteIconSection removes a section with its icons.
func (s *Server) deleteIconSection(w http.ResponseWriter, r *http.Request, lib iconLibrary) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	n, err := s.store.DeleteIconSection(r.Context(), lib.id, id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		s.internalError(w, r, "deleting icon section", err)
		return
	}
	if lib.id == store.ServerIcons {
		s.audit(r, "icons.section.delete", strconv.FormatInt(id, 10), strconv.FormatInt(n, 10)+" icons")
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"icons": n})
}

func (s *Server) handleDeleteIconSection(w http.ResponseWriter, r *http.Request) {
	s.deleteIconSection(w, r, s.somaLibrary(r))
}
func (s *Server) handleAdminDeleteIconSection(w http.ResponseWriter, r *http.Request) {
	s.deleteIconSection(w, r, serverLibrary)
}
