package server

import (
	"context"
	"net/http"
	"time"

	"dendronotes/internal/httpx"
)

// ldapStatus describes the directory connection for the administration area.
type ldapStatus struct {
	URL         string    `json:"url"`
	BaseDN      string    `json:"baseDN"`
	BindDN      string    `json:"bindDN"`
	GroupBaseDN string    `json:"groupBaseDN"`
	Ready       bool      `json:"ready"`
	CheckedAt   time.Time `json:"checkedAt"`
	Error       string    `json:"error,omitempty"`
}

// checkLDAP connects with the service account and remembers the result.
func (s *Server) checkLDAP(ctx context.Context) ldapStatus {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	l := s.cfg.Auth.LDAP
	st := ldapStatus{URL: l.URL, BaseDN: l.BaseDN, BindDN: l.BindDN, GroupBaseDN: l.GroupBaseDN, CheckedAt: time.Now().UTC()}
	if err := s.ldap.Check(ctx); err != nil {
		st.Error = err.Error()
		s.log.Warn("directory check failed", "err", err)
	} else {
		st.Ready = true
	}
	s.ldapMu.Lock()
	s.ldapLast = st
	s.ldapMu.Unlock()
	return st
}

// ldapState is the last known directory status (nil without LDAP).
func (s *Server) ldapState() *ldapStatus {
	if s.ldap == nil {
		return nil
	}
	s.ldapMu.Lock()
	defer s.ldapMu.Unlock()
	st := s.ldapLast
	if st.URL == "" { // not checked yet
		l := s.cfg.Auth.LDAP
		st = ldapStatus{URL: l.URL, BaseDN: l.BaseDN, BindDN: l.BindDN, GroupBaseDN: l.GroupBaseDN}
	}
	return &st
}

// handleAdminLDAPCheck tests the directory connection again.
func (s *Server) handleAdminLDAPCheck(w http.ResponseWriter, r *http.Request) {
	if s.ldap == nil {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s.checkLDAP(r.Context()))
}

// publishGroupsAccess tells the open clients of the somas shared with these
// groups that their members changed (directory sync at sign-in).
func (s *Server) publishGroupsAccess(ctx context.Context, groups []int64) {
	var somaIDs []string
	for _, id := range groups {
		ids, err := s.store.GroupSomas(ctx, id)
		if err != nil {
			s.log.Error("listing group somas", "group", id, "err", err)
			continue
		}
		somaIDs = append(somaIDs, ids...)
	}
	s.publishAccess(somaIDs)
}
