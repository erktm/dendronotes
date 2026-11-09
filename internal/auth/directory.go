package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"dendronotes/internal/httpx"
	"dendronotes/internal/ldap"
	"dendronotes/internal/store"
)

// Directory checks a sign-in against Active Directory / LDAP (internal/ldap).
type Directory interface {
	Authenticate(ctx context.Context, login, password string) (ldap.Entry, error)
}

// directoryTimeout bounds one directory sign-in (search, bind, groups).
const directoryTimeout = 20 * time.Second

// directoryLogin signs in an account the directory vouches for: it creates or
// updates the account (name, e-mail, admin role from groups), syncs its
// groups below the group base and starts the session. Local accounts never
// get here (HandleLogin checks them first).
func (a *Authenticator) directoryLogin(w http.ResponseWriter, r *http.Request, login, password, ip string, fail func(string)) {
	ctx, cancel := context.WithTimeout(r.Context(), directoryTimeout)
	defer cancel()
	e, err := a.cfg.Directory.Authenticate(ctx, login, password)
	switch {
	case errors.Is(err, ldap.ErrInvalidCredentials):
		fail("credentials (directory)")
		return
	case err != nil:
		a.log.Error("directory sign-in failed", "user", login, "err", err)
		a.store.Audit(r.Context(), store.AuditEntry{Actor: login, Action: "login.failed", Detail: "directory unavailable", IP: ip})
		httpx.Error(w, http.StatusServiceUnavailable, "directory_unavailable")
		return
	}
	if len(a.cfg.DirectoryAllowedGroups) > 0 && !e.InAny(a.cfg.DirectoryAllowedGroups) {
		a.log.Warn("directory account not in an allowed group", "ip", ip, "user", e.Username)
		a.store.Audit(r.Context(), store.AuditEntry{Actor: e.Username, Action: "login.failed", Detail: "not in an allowed group", IP: ip})
		httpx.Error(w, http.StatusForbidden, "account_not_allowed")
		return
	}
	syncRole := len(a.cfg.DirectoryAdminGroups) > 0
	u, err := a.externalAccount(r.Context(), external{
		via: "directory", source: store.SourceLDAP, ldapID: e.ID,
		username: e.Username, name: e.Name, email: e.Email,
		syncRole: syncRole, admin: syncRole && e.InAny(a.cfg.DirectoryAdminGroups),
		lookup: func(ctx context.Context) (store.User, error) { return a.store.UserByLDAP(ctx, e.ID) },
	}, ip)
	if err != nil {
		a.log.Error("directory account failed", "user", e.Username, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if a.cfg.DirectoryGroups && !u.Disabled {
		var groups []store.DirectoryGroup
		for _, g := range e.Groups {
			if g.Synced {
				groups = append(groups, store.DirectoryGroup{ID: g.ID, Name: g.Name})
			}
		}
		// A failed sync must not lock people out; the next sign-in tries again.
		if changed, err := a.store.SyncDirectoryGroups(r.Context(), u.ID, groups); err != nil {
			a.log.Error("syncing directory groups failed", "user", u.Username, "err", err)
		} else if len(changed) > 0 && a.cfg.OnDirectoryGroups != nil {
			a.cfg.OnDirectoryGroups(r.Context(), changed)
		}
	}
	a.startSession(w, r, u, ip, "directory")
}
