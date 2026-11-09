package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"dendronotes/internal/auth"
	"dendronotes/internal/events"
	"dendronotes/internal/httpx"
	"dendronotes/internal/store"
)

// Sharing: the owner of a soma decides who else may use it and how – people
// and groups, each as viewer, writer, contributor or editor (store.RoleRank).
// Members see who has access; people may leave a soma shared with them
// personally.

// accessChanged tells the soma's connected clients that members, roles or the
// soma itself changed, so they reload their soma list and role.
func accessChanged(r *http.Request) {
	sm, _ := somaOf(r)
	runtimeOf(r).Hub.Publish(events.Event{Name: "access", Data: map[string]string{"soma": sm.ID}})
}

// handleMembers lists the people and groups with access to a soma.
func (s *Server) handleMembers(w http.ResponseWriter, r *http.Request) {
	sm, role := somaOf(r)
	list, err := s.store.Members(r.Context(), sm.ID)
	if err != nil {
		s.internalError(w, r, "listing members", err)
		return
	}
	groups, err := s.store.SomaGroups(r.Context(), sm.ID)
	if err != nil {
		s.internalError(w, r, "listing groups", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"members": list, "groups": groups, "role": role})
}

// dropIfNoAccess ends the live streams of an account in the request's soma
// unless it still has access (for example through a group).
func (s *Server) dropIfNoAccess(r *http.Request, userID int64) {
	sm, _ := somaOf(r)
	if role, err := s.store.EffectiveRole(r.Context(), sm.ID, userID); err == nil && role == "" {
		runtimeOf(r).DropUser(userID)
	}
}

// memberTarget resolves {id} to an enabled account other than the caller.
func (s *Server) memberTarget(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusBadRequest, "unknown_user")
		return store.User{}, false
	}
	if me, _ := auth.UserFrom(r.Context()); me.ID == id {
		httpx.Error(w, http.StatusBadRequest, "self")
		return store.User{}, false
	}
	u, err := s.store.UserByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusBadRequest, "unknown_user")
		return store.User{}, false
	}
	if err != nil {
		s.internalError(w, r, "user lookup", err)
		return store.User{}, false
	}
	return u, true
}

// handleSetMember shares the soma with an account or changes its role.
func (s *Server) handleSetMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Role string `json:"role"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if !store.IsShareRole(body.Role) {
		httpx.Error(w, http.StatusBadRequest, "invalid_role")
		return
	}
	u, ok := s.memberTarget(w, r)
	if !ok {
		return
	}
	if u.Disabled {
		httpx.Error(w, http.StatusBadRequest, "unknown_user")
		return
	}
	sm, _ := somaOf(r)
	if err := s.store.SetMember(r.Context(), sm.ID, u.ID, body.Role); err != nil {
		s.internalError(w, r, "setting member", err)
		return
	}
	s.audit(r, "member.set", sm.ID, u.Username+": "+body.Role)
	accessChanged(r)
	s.handleMembers(w, r)
}

// handleRemoveMember takes an account's access away.
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	u, ok := s.memberTarget(w, r)
	if !ok {
		return
	}
	sm, _ := somaOf(r)
	if err := s.store.RemoveMember(r.Context(), sm.ID, u.ID); err != nil {
		s.internalError(w, r, "removing member", err)
		return
	}
	s.audit(r, "member.remove", sm.ID, u.Username)
	accessChanged(r)
	s.dropIfNoAccess(r, u.ID) // no more live updates of this soma
	s.handleMembers(w, r)
}

// handleLeaveSoma removes the caller from a soma shared with them personally
// (access through a group stays).
func (s *Server) handleLeaveSoma(w http.ResponseWriter, r *http.Request) {
	sm, _ := somaOf(r)
	u, _ := auth.UserFrom(r.Context())
	direct, err := s.store.MemberRole(r.Context(), sm.ID, u.ID)
	if err != nil {
		s.internalError(w, r, "membership lookup", err)
		return
	}
	switch direct {
	case store.MemberOwner:
		httpx.Error(w, http.StatusBadRequest, "owner_cannot_leave")
		return
	case "":
		httpx.Error(w, http.StatusBadRequest, "not_shared_personally")
		return
	}
	if err := s.store.RemoveMember(r.Context(), sm.ID, u.ID); err != nil {
		s.internalError(w, r, "leaving soma", err)
		return
	}
	s.audit(r, "member.leave", sm.ID, u.Username)
	accessChanged(r)
	s.dropIfNoAccess(r, u.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": sm.ID})
}

// handleSearchUsers finds accounts to share a soma with. It reveals only user
// and display names, and only for at least two characters.
func (s *Server) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	people := []store.Person{}
	groups := []store.Group{}
	if utf8.RuneCountInString(q) < 2 || len(q) > 100 {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": people, "groups": groups})
		return
	}
	found, err := s.store.SearchUsers(r.Context(), q, 10)
	if err != nil {
		s.internalError(w, r, "searching users", err)
		return
	}
	for _, u := range found {
		people = append(people, store.Person{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName})
	}
	if groups, err = s.store.SearchGroups(r.Context(), q, 10); err != nil {
		s.internalError(w, r, "searching groups", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": people, "groups": groups})
}

// groupTarget resolves {id} to a group.
func (s *Server) groupTarget(w http.ResponseWriter, r *http.Request) (store.Group, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Error(w, http.StatusNotFound, "unknown_group")
		return store.Group{}, false
	}
	g, err := s.store.GroupByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "unknown_group")
		return store.Group{}, false
	}
	if err != nil {
		s.internalError(w, r, "group lookup", err)
		return store.Group{}, false
	}
	return g, true
}

// handleSetSomaGroup shares the soma with a group or changes the group's role.
func (s *Server) handleSetSomaGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Role string `json:"role"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 1024); err != nil {
		httpx.WriteDecodeError(w, err)
		return
	}
	if !store.IsShareRole(body.Role) {
		httpx.Error(w, http.StatusBadRequest, "invalid_role")
		return
	}
	g, ok := s.groupTarget(w, r)
	if !ok {
		return
	}
	sm, _ := somaOf(r)
	if err := s.store.SetSomaGroup(r.Context(), sm.ID, g.ID, body.Role); err != nil {
		s.internalError(w, r, "sharing with group", err)
		return
	}
	s.audit(r, "member.group.set", sm.ID, g.Name+": "+body.Role)
	accessChanged(r)
	s.handleMembers(w, r)
}

// handleRemoveSomaGroup stops sharing the soma with a group; the live streams
// of people who lost access end (live.go).
func (s *Server) handleRemoveSomaGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := s.groupTarget(w, r)
	if !ok {
		return
	}
	sm, _ := somaOf(r)
	if err := s.store.RemoveSomaGroup(r.Context(), sm.ID, g.ID); err != nil {
		s.internalError(w, r, "unsharing with group", err)
		return
	}
	s.audit(r, "member.group.remove", sm.ID, g.Name)
	accessChanged(r)
	s.handleMembers(w, r)
}
