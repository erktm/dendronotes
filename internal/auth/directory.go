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
	// Lookup finds an account again by its id (ldap.ErrNotFound if gone).
	Lookup(ctx context.Context, id string) (ldap.Entry, error)
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
		a.counted("directory", false)
		a.log.Error("directory sign-in failed", "user", login, "err", err)
		a.store.Audit(r.Context(), store.AuditEntry{Actor: login, Action: "login.failed", Detail: "directory unavailable", IP: ip})
		httpx.Error(w, http.StatusServiceUnavailable, "directory_unavailable")
		return
	}
	if len(a.cfg.DirectoryAllowedGroups) > 0 && !e.InAny(a.cfg.DirectoryAllowedGroups) {
		a.counted("directory", false)
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

// DirectorySync is what a sync with the directory did.
type DirectorySync struct {
	Accounts  int // directory accounts checked
	Updated   int // with a new name, e-mail, role or groups
	SignedOut int // signed out (and API tokens revoked): gone, disabled or not allowed
}

// ErrSuspiciousSync stops a sync that would sign out most directory accounts
// at once – more likely a wrong base DN or filter than real departures.
var ErrSuspiciousSync = errors.New("most directory accounts were not found – nothing was signed out; check the LDAP settings")

// SyncDirectory checks every directory account against the directory, in
// the background: an account that is gone, disabled there or no longer in an
// allowed group is signed out everywhere and loses its API tokens (its next
// sign-in fails anyway; the account itself stays). The others take over name,
// e-mail, the admin role (with admin groups) and their groups. When the
// directory cannot be reached, the sync stops and changes nothing more.
func (a *Authenticator) SyncDirectory(ctx context.Context) (DirectorySync, error) {
	var res DirectorySync
	if a.cfg.Directory == nil {
		return res, nil
	}
	users, err := a.store.Users(ctx)
	if err != nil {
		return res, err
	}
	type found struct {
		u     store.User
		e     ldap.Entry
		leave string // reason to sign out, or ""
	}
	var all []found
	gone := 0
	for _, u := range users {
		if u.Source != store.SourceLDAP || u.LDAPID == "" {
			continue
		}
		res.Accounts++
		e, err := a.cfg.Directory.Lookup(ctx, u.LDAPID)
		switch {
		case errors.Is(err, ldap.ErrNotFound):
			gone++
			all = append(all, found{u: u, leave: "not in the directory"})
		case err != nil:
			return res, err
		case e.Disabled:
			all = append(all, found{u: u, e: e, leave: "disabled in the directory"})
		case len(a.cfg.DirectoryAllowedGroups) > 0 && !e.InAny(a.cfg.DirectoryAllowedGroups):
			all = append(all, found{u: u, e: e, leave: "not in an allowed group"})
		default:
			all = append(all, found{u: u, e: e})
		}
	}
	if res.Accounts >= 3 && gone*2 > res.Accounts {
		return res, ErrSuspiciousSync
	}
	for _, f := range all {
		if f.leave != "" {
			if a.signOutAccount(ctx, f.u, f.leave) {
				res.SignedOut++
			}
			continue
		}
		if a.updateFromDirectory(ctx, f.u, f.e) {
			res.Updated++
		}
	}
	return res, nil
}

// signOutAccount ends the sessions and revokes the API tokens of an account
// the directory no longer vouches for; it reports whether there were any.
func (a *Authenticator) signOutAccount(ctx context.Context, u store.User, reason string) bool {
	sessions, err := a.store.EndUserSessions(ctx, u.ID, 0)
	if err != nil {
		a.log.Error("ending sessions failed", "user", u.Username, "err", err)
	}
	tokens, err := a.store.DeleteUserTokens(ctx, u.ID)
	if err != nil {
		a.log.Error("revoking tokens failed", "user", u.Username, "err", err)
	}
	if sessions+tokens == 0 {
		return false
	}
	a.log.Info("directory account signed out", "user", u.Username, "reason", reason, "sessions", sessions, "tokens", tokens)
	a.store.Audit(ctx, store.AuditEntry{Actor: "directory sync", Action: "user.signout", Target: u.Username, Detail: reason})
	if a.cfg.OnSignedOut != nil {
		a.cfg.OnSignedOut(ctx, u.ID)
	}
	return true
}

// updateFromDirectory takes over name, e-mail, role and groups of an
// account; it reports whether anything changed.
func (a *Authenticator) updateFromDirectory(ctx context.Context, u store.User, e ldap.Entry) bool {
	syncRole := len(a.cfg.DirectoryAdminGroups) > 0
	after, err := a.externalAccount(ctx, external{
		via: "directory sync", source: store.SourceLDAP, ldapID: u.LDAPID,
		username: e.Username, name: e.Name, email: e.Email,
		syncRole: syncRole, admin: syncRole && e.InAny(a.cfg.DirectoryAdminGroups),
		lookup: func(context.Context) (store.User, error) { return u, nil },
	}, "")
	if err != nil {
		a.log.Error("updating directory account failed", "user", u.Username, "err", err)
		return false
	}
	changed := after.DisplayName != u.DisplayName || after.Email != u.Email || after.Role != u.Role
	if a.cfg.DirectoryGroups && !u.Disabled {
		var groups []store.DirectoryGroup
		for _, g := range e.Groups {
			if g.Synced {
				groups = append(groups, store.DirectoryGroup{ID: g.ID, Name: g.Name})
			}
		}
		ids, err := a.store.SyncDirectoryGroups(ctx, u.ID, groups)
		if err != nil {
			a.log.Error("syncing directory groups failed", "user", u.Username, "err", err)
		} else if len(ids) > 0 {
			changed = true
			if a.cfg.OnDirectoryGroups != nil {
				a.cfg.OnDirectoryGroups(ctx, ids)
			}
		}
	}
	return changed
}
