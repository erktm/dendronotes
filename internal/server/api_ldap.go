package server

import (
	"context"
	"fmt"
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
	// Sync is the state of the sync in the background.
	Sync *ldapSyncStatus `json:"sync"`
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
	sync := s.ldapSync
	st.Sync = &sync
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
	sync := s.ldapSync
	st.Sync = &sync
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

// ldapSyncStatus is the state of the directory sync in the background.
type ldapSyncStatus struct {
	Interval    string    `json:"interval"` // empty: off
	Running     bool      `json:"running"`
	LastRun     time.Time `json:"lastRun,omitzero"`
	LastSuccess time.Time `json:"lastSuccess,omitzero"`
	Accounts    int       `json:"accounts"`
	Updated     int       `json:"updated"`
	SignedOut   int       `json:"signedOut"`
	Error       string    `json:"error,omitempty"`
}

// ldapSyncState is the sync state (nil without LDAP).
func (s *Server) ldapSyncState() *ldapSyncStatus {
	if s.ldap == nil {
		return nil
	}
	s.ldapMu.Lock()
	defer s.ldapMu.Unlock()
	st := s.ldapSync
	return &st
}

// runLDAPSync checks all directory accounts once (see auth.SyncDirectory);
// ok is false when a sync is already running.
func (s *Server) runLDAPSync(ctx context.Context) (st ldapSyncStatus, ok bool) {
	s.ldapMu.Lock()
	if s.ldapSync.Running {
		st = s.ldapSync
		s.ldapMu.Unlock()
		return st, false
	}
	s.ldapSync.Running = true
	s.ldapMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	res, err := s.auth.SyncDirectory(ctx)

	s.ldapMu.Lock()
	defer s.ldapMu.Unlock()
	st = s.ldapSync
	st.Running, st.LastRun = false, time.Now().UTC()
	st.Accounts, st.Updated, st.SignedOut, st.Error = res.Accounts, res.Updated, res.SignedOut, ""
	if err != nil {
		st.Error = err.Error()
		s.log.Warn("directory sync failed", "err", err)
	} else {
		st.LastSuccess = st.LastRun
		if res.Updated+res.SignedOut > 0 {
			s.log.Info("directory synced", "accounts", res.Accounts, "updated", res.Updated, "signed_out", res.SignedOut)
		}
	}
	s.ldapSync = st
	return st, true
}

// syncLDAPEvery runs the sync in the background until ctx ends; the first
// run is a minute after the start.
func (s *Server) syncLDAPEvery(ctx context.Context, every time.Duration) {
	wait := time.NewTimer(time.Minute)
	defer wait.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-wait.C:
			s.runLDAPSync(ctx)
			wait.Reset(every)
		}
	}
}

// handleAdminLDAPSync runs the sync now and answers with its result.
func (s *Server) handleAdminLDAPSync(w http.ResponseWriter, r *http.Request) {
	if s.ldap == nil {
		httpx.Error(w, http.StatusNotFound, "not_available")
		return
	}
	st, ok := s.runLDAPSync(r.Context())
	if !ok {
		httpx.Error(w, http.StatusConflict, "sync_running")
		return
	}
	s.audit(r, "directory.sync", "", fmt.Sprintf("%d accounts, %d updated, %d signed out", st.Accounts, st.Updated, st.SignedOut))
	httpx.WriteJSON(w, http.StatusOK, st)
}

// dropUserStreams ends the live streams of an account in every open soma.
func (s *Server) dropUserStreams(_ context.Context, userID int64) {
	for _, rt := range s.somas.Running() {
		rt.DropUser(userID)
	}
}
