package server

import (
	"errors"
	"net/http"
	"strconv"

	"dendronotes/internal/events"
	"dendronotes/internal/httpx"
	"dendronotes/internal/somas"
	"dendronotes/internal/store"
)

// Groups (admin area): sets of accounts somas can be shared with. Everyone is
// built in – every enabled account – and cannot be renamed, deleted or given
// members.

// publishAccess tells the open clients of these somas that sharing changed;
// streams of accounts that lost access end (live.go).
func (s *Server) publishAccess(ids []string) {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	for _, rt := range s.somas.Running() {
		if set[rt.ID] {
			rt.Hub.Publish(events.Event{Name: "access", Data: map[string]string{"soma": rt.ID}})
		}
	}
}

// groupError maps store errors of group changes to responses.
func (s *Server) groupError(w http.ResponseWriter, r *http.Request, what string, err error) {
	switch {
	case errors.Is(err, store.ErrBuiltinGroup):
		httpx.Error(w, http.StatusBadRequest, "builtin_group")
	case errors.Is(err, store.ErrDirectoryGroup):
		httpx.Error(w, http.StatusBadRequest, "directory_group")
	case errors.Is(err, store.ErrTaken):
		httpx.Error(w, http.StatusConflict, "name_taken")
	case errors.Is(err, store.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found")
	default:
		s.internalError(w, r, what, err)
	}
}

func (s *Server) handleAdminGroups(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.Groups(r.Context())
	if err != nil {
		s.internalError(w, r, "listing groups", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"groups": list})
}

// groupName reads and checks {"name": …} (the rules of soma names).
func groupName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(w, r, &body, 4096); err != nil {
		httpx.WriteDecodeError(w, err)
		return "", false
	}
	name, err := somas.CleanName(body.Name)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_name")
		return "", false
	}
	return name, true
}

func (s *Server) handleAdminCreateGroup(w http.ResponseWriter, r *http.Request) {
	name, ok := groupName(w, r)
	if !ok {
		return
	}
	g, err := s.store.CreateGroup(r.Context(), name)
	if err != nil {
		s.groupError(w, r, "creating group", err)
		return
	}
	s.audit(r, "group.create", g.Name, "")
	httpx.WriteJSON(w, http.StatusCreated, g)
}

func (s *Server) handleAdminRenameGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := s.groupTarget(w, r)
	if !ok {
		return
	}
	name, ok := groupName(w, r)
	if !ok {
		return
	}
	ng, err := s.store.RenameGroup(r.Context(), g.ID, name)
	if err != nil {
		s.groupError(w, r, "renaming group", err)
		return
	}
	s.audit(r, "group.rename", g.Name, ng.Name)
	httpx.WriteJSON(w, http.StatusOK, ng)
}

func (s *Server) handleAdminDeleteGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := s.groupTarget(w, r)
	if !ok {
		return
	}
	shared, err := s.store.GroupSomas(r.Context(), g.ID)
	if err != nil {
		s.internalError(w, r, "listing group somas", err)
		return
	}
	if err := s.store.DeleteGroup(r.Context(), g.ID); err != nil {
		s.groupError(w, r, "deleting group", err)
		return
	}
	s.audit(r, "group.delete", g.Name, "")
	s.publishAccess(shared)
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"id": g.ID})
}

func (s *Server) handleAdminGroupMembers(w http.ResponseWriter, r *http.Request) {
	g, ok := s.groupTarget(w, r)
	if !ok {
		return
	}
	list, err := s.store.GroupMembers(r.Context(), g.ID)
	if err != nil {
		s.groupError(w, r, "listing group members", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"group": g, "members": list})
}

// groupMemberChange adds ({user} in) or removes an account and answers with
// the group's members.
func (s *Server) groupMemberChange(w http.ResponseWriter, r *http.Request, add bool) {
	g, ok := s.groupTarget(w, r)
	if !ok {
		return
	}
	uid, err := strconv.ParseInt(r.PathValue("user"), 10, 64)
	if err != nil || uid <= 0 {
		httpx.Error(w, http.StatusBadRequest, "unknown_user")
		return
	}
	u, err := s.store.UserByID(r.Context(), uid)
	if errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, http.StatusBadRequest, "unknown_user")
		return
	}
	if err != nil {
		s.internalError(w, r, "user lookup", err)
		return
	}
	action := "group.member.add"
	if add {
		err = s.store.AddGroupMember(r.Context(), g.ID, u.ID)
	} else {
		action = "group.member.remove"
		err = s.store.RemoveGroupMember(r.Context(), g.ID, u.ID)
	}
	if err != nil {
		s.groupError(w, r, "changing group members", err)
		return
	}
	s.audit(r, action, g.Name, u.Username)
	if shared, err := s.store.GroupSomas(r.Context(), g.ID); err == nil {
		s.publishAccess(shared)
	}
	s.handleAdminGroupMembers(w, r)
}

func (s *Server) handleAdminAddGroupMember(w http.ResponseWriter, r *http.Request) {
	s.groupMemberChange(w, r, true)
}

func (s *Server) handleAdminRemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	s.groupMemberChange(w, r, false)
}
