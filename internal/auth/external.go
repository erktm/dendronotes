package auth

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"dendronotes/internal/store"
)

// external is an account another system vouches for: an OpenID Connect
// provider or the directory (Active Directory / LDAP).
type external struct {
	via             string // "sso" or "directory" (logs, audit)
	source          store.Source
	issuer, subject string // SourceOIDC
	ldapID          string // SourceLDAP
	username        string // wish for the user name
	name, email     string
	syncRole, admin bool // with admin groups configured, the role follows them
	lookup          func(context.Context) (store.User, error)
}

// externalAccount returns the account of an external identity, creating it
// on the first sign-in. Name, e-mail and (with admin groups configured) the
// role follow the other system on every sign-in. Accounts are matched by
// their stable id only, never by name: a local account "erik" is not taken
// over by an external user "erik", who gets "erik-2" instead.
func (a *Authenticator) externalAccount(ctx context.Context, x external, ip string) (store.User, error) {
	role := store.RoleUser
	if x.admin {
		role = store.RoleAdmin
	}
	name := truncate(strings.TrimSpace(x.name), 100)
	email := truncate(strings.TrimSpace(x.email), 254)

	u, err := x.lookup(ctx)
	if err == nil {
		var up store.UserUpdate
		changed := false
		if name != "" && name != u.DisplayName {
			up.DisplayName, changed = &name, true
		}
		if email != "" && email != u.Email {
			up.Email, changed = &email, true
		}
		if x.syncRole && role != u.Role {
			up.Role, changed = &role, true
			a.store.Audit(ctx, store.AuditEntry{Actor: "system", Action: "user.update", Target: u.Username,
				Detail: "role " + string(role) + " from " + x.via + " groups", IP: ip})
		}
		if !changed {
			return u, nil
		}
		return a.store.UpdateUser(ctx, u.ID, up)
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.User{}, err
	}

	base := usernameFrom(x.username)
	if base == "" {
		base = usernameFrom(strings.Split(x.email, "@")[0])
	}
	if base == "" {
		base = "user"
	}
	for i := 1; i <= 50; i++ {
		candidate := base
		if i > 1 {
			candidate = base + "-" + strconv.Itoa(i)
		}
		u, err = a.store.CreateUser(ctx, store.User{
			Username: candidate, DisplayName: name, Email: email, Role: role, Source: x.source,
			OIDCIssuer: x.issuer, OIDCSubject: x.subject, LDAPID: x.ldapID,
		})
		if !errors.Is(err, store.ErrTaken) {
			break
		}
		// Taken may also mean the identity was just created by a parallel sign-in.
		if existing, lerr := x.lookup(ctx); lerr == nil {
			return existing, nil
		}
	}
	if err != nil {
		return store.User{}, err
	}
	a.log.Info("account created on first sign-in", "user", u.Username, "via", x.via)
	a.store.Audit(ctx, store.AuditEntry{ActorID: u.ID, Actor: u.Username, Action: "user.provision", Detail: x.via, IP: ip})
	if a.cfg.OnProvision != nil {
		if err := a.cfg.OnProvision(ctx, u); err != nil {
			a.log.Error("preparing new account failed", "user", u.Username, "err", err)
		}
	}
	return u, nil
}
