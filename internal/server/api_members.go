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

// Sharing: the owner of a soma decides who else may read (viewer) or change
// (editor) it. Members see who has access; anyone may leave a shared soma.

// accessChanged tells the soma's connected clients that members, roles or the
// soma itself changed, so they reload their soma list and role.
func accessChanged(r *http.Request) {
	sm, _ := somaOf(r)
	runtimeOf(r).Hub.Publish(events.Event{Name: "access", Data: map[string]string{"soma": sm.ID}})
}

// handleMembers lists the people with access to a soma.
func (s *Server) handleMembers(w http.ResponseWriter, r *http.Request) {
	sm, role := somaOf(r)
	list, err := s.store.Members(r.Context(), sm.ID)
	if err != nil {
		s.internalError(w, r, "listing members", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"members": list, "role": role})
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
	if body.Role != store.MemberEditor && body.Role != store.MemberViewer {
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
	s.handleMembers(w, r)
}

// handleLeaveSoma removes the caller from a soma shared with them.
func (s *Server) handleLeaveSoma(w http.ResponseWriter, r *http.Request) {
	sm, role := somaOf(r)
	if role == store.MemberOwner {
		httpx.Error(w, http.StatusBadRequest, "owner_cannot_leave")
		return
	}
	u, _ := auth.UserFrom(r.Context())
	if err := s.store.RemoveMember(r.Context(), sm.ID, u.ID); err != nil {
		s.internalError(w, r, "leaving soma", err)
		return
	}
	s.audit(r, "member.leave", sm.ID, u.Username)
	accessChanged(r)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": sm.ID})
}

// handleSearchUsers finds accounts to share a soma with. It reveals only user
// and display names, and only for at least two characters.
func (s *Server) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	people := []store.Person{}
	if utf8.RuneCountInString(q) < 2 || len(q) > 100 {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": people})
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
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": people})
}
