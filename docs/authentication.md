# Authentication

dendronotes signs people in with its own accounts and passwords, through an authenticating
proxy, through Keycloak or another OpenID Connect provider, or with Active Directory / LDAP.
All settings are environment variables; the full list is in the
[README](../README.md#configuration).

**Accounts with passwords** (default, `local`). On the first start dendronotes creates the
admin account `DENDRONOTES_ADMIN_USERNAME` with the password from one of the admin password
variables. Create a hash for it and pass it as a secret:

```bash
docker run --rm -it dendronotes:<version> hash-password
```

Further accounts are created by an admin in the administration area. People sign in with user
name or e-mail address and password (at least 10 characters) and can change the password in
their profile. An e-mail address only works when exactly one account with a password has it.
Passwords are stored as argon2id hashes. Sign-in attempts are rate-limited per client IP, per
account and globally; sessions are stored only as SHA-256 hashes in `state.db` and sent as
`HttpOnly`, `SameSite=Strict` cookies (`__Host-` prefixed and `Secure` over HTTPS).

**Behind an authenticating proxy** (e.g. oauth2-proxy with Keycloak): set
`DENDRONOTES_AUTH_MODE=proxy`, `DENDRONOTES_TRUSTED_PROXIES` to the proxy's address range and
`DENDRONOTES_AUTH_PROXY_HEADER` to the header it sets (oauth2-proxy with
`--set-xauthrequest`: `X-Auth-Request-User`). The header is ignored from any other peer, so make
sure only the proxy can reach dendronotes (Kubernetes: a NetworkPolicy). Each user name gets an
account on first access; the one named `DENDRONOTES_ADMIN_USERNAME` becomes admin while there is
no admin yet.

**Through Keycloak or another OpenID Connect provider** (in local mode, next to passwords).
Register a client in Keycloak:

- Client type *OpenID Connect*, *Standard flow* on; *Client authentication* on (then put the
  secret into a file for `DENDRONOTES_OIDC_CLIENT_SECRET_FILE`) or off for a public client;
  dendronotes always uses PKCE.
- *Valid redirect URIs*: `https://notes.example.org/api/auth/oidc/callback`;
  *Valid post logout redirect URIs*: `https://notes.example.org/` (with your
  `DENDRONOTES_PUBLIC_URL`).
- For groups: a *Group Membership* mapper in the client's dedicated scope, token claim name
  `groups`, added to the ID token or the userinfo response ("Full group path" on or off both
  work: a leading `/` is ignored). Realm roles work too with
  `DENDRONOTES_OIDC_GROUPS_CLAIM=realm_access.roles`.

```bash
-e DENDRONOTES_PUBLIC_URL=https://notes.example.org \
-e DENDRONOTES_OIDC_ISSUER=https://sso.example.org/realms/main \
-e DENDRONOTES_OIDC_CLIENT_ID=dendronotes \
-e DENDRONOTES_OIDC_CLIENT_SECRET_FILE=/run/secrets/oidc-client-secret \
-e DENDRONOTES_OIDC_ADMIN_GROUPS=dendronotes-admins \
-e DENDRONOTES_OIDC_LOGIN_LABEL=Keycloak
```

The sign-in page then offers "Continue with Keycloak". The first sign-in creates an account
(with an empty soma "Notes"); accounts are matched by the provider's issuer and subject, never
by name, so a provider user called like an existing local account gets a name with a suffix
(`erik-2`) instead of taking it over. Display name and e-mail follow the provider on every
sign-in; with `DENDRONOTES_OIDC_ADMIN_GROUPS` set, so does the admin role (without it, roles are
managed in the administration area). `DENDRONOTES_OIDC_ALLOWED_GROUPS` keeps everyone else out.
"Sign out" also ends the Keycloak session. dendronotes keeps its own session afterwards
(`DENDRONOTES_SESSION_TTL`): taking someone's access away in Keycloak takes effect at their next
sign-in. To end it at once, disable the account or end its sessions in the administration area.

With `DENDRONOTES_LOCAL_LOGIN=false` the sign-in page shows only the Keycloak button; the admin
account can still sign in with its password at `/?local=1` should the provider be unreachable.
The administration area shows whether the provider is reachable.

**With Active Directory** (or another LDAP directory; in local mode, next to or instead of
passwords). People sign in on the normal form with their directory account (user name
`sAMAccountName`, `user@domain` as UPN, or e-mail) and its password. dendronotes looks the
account up with a read-only service account, then checks the password by binding as that
account; it never stores the password. Local accounts come first: when what was typed matches a
local account with a password, only that password counts.

```bash
-e DENDRONOTES_LDAP_URL=ldaps://dc1.example.org -e DENDRONOTES_LDAP_BIND_DN='CN=svc-dendronotes,OU=Service,DC=example,DC=org' -e DENDRONOTES_LDAP_BIND_PASSWORD_FILE=/run/secrets/ldap-bind-password -e DENDRONOTES_LDAP_BASE_DN='DC=example,DC=org' -e DENDRONOTES_LDAP_GROUP_BASE_DN='OU=dendronotes,OU=Groups,DC=example,DC=org' -e DENDRONOTES_LDAP_ADMIN_GROUPS='dendronotes-admins' -e DENDRONOTES_LDAP_ALLOWED_GROUPS='dendronotes-users;dendronotes-admins'
```

- **Connection**: `ldaps://` (port 636), or `ldap://` with `DENDRONOTES_LDAP_START_TLS=true`.
  Certificates are verified; add your own CA with `DENDRONOTES_LDAP_CA_FILE`. Unencrypted
  `ldap://` is refused except for `localhost`.
- **Accounts** are created at the first sign-in (with an empty soma "Notes") and bound to the
  directory's `objectGUID`, never to the name; a directory account called like an existing local
  account gets a suffix (`erik-2`). Display name and e-mail follow the directory on every
  sign-in and cannot be changed in dendronotes; there is no dendronotes password.
- **Admins and access**: with `DENDRONOTES_LDAP_ADMIN_GROUPS`, membership decides the admin role
  at every sign-in (without it, roles are managed in the administration area);
  `DENDRONOTES_LDAP_ALLOWED_GROUPS` keeps everyone else out. Groups are given by name (`cn`) or
  DN; nested groups count.
- **Groups**: every group below `DENDRONOTES_LDAP_GROUP_BASE_DN` that someone is a member of,
  directly or through nested groups, becomes a dendronotes group at their sign-in, so somas can
  be shared with it like with any group. Name and members follow the directory: someone joins
  or leaves a group at their next sign-in. In the administration area these groups are marked
  *Active Directory*; they can be deleted (the next member's sign-in brings them back) but not
  renamed or given other members. Keep the group base narrow (a dedicated OU) rather than the
  whole domain. A local group with the same name stays separate; the directory group is shown
  as "Name (AD)".
- **Sync in the background** (`DENDRONOTES_LDAP_SYNC_INTERVAL`, every 15 minutes; `0` turns it
  off): every directory account is looked up again by its `objectGUID`. Accounts that are gone,
  disabled in Active Directory or no longer in `LDAP_ALLOWED_GROUPS` are signed out everywhere
  and lose their API tokens; the account itself stays, its next sign-in fails. The others take
  over name, e-mail, the admin role and their groups without signing in. If the directory cannot
  be reached nothing changes, and if most accounts are missing at once nobody is signed out (a
  wrong base DN or filter is more likely); the administration area shows the last result and
  offers *Sync now*. Without the sync, changes in Active Directory take effect at the next
  sign-in, and dendronotes keeps its own session until then (`DENDRONOTES_SESSION_TTL`). With
  `DENDRONOTES_LOCAL_LOGIN=false` only directory accounts and the admin account sign in with a
  password.
- The service account needs read access only. The administration area (*Server*) shows whether
  the directory is reachable and checks the connection on request.

Other LDAP directories work with their own filters and attributes, for example OpenLDAP:

```bash
-e DENDRONOTES_LDAP_USER_FILTER='(&(objectClass=inetOrgPerson)(|(uid={login})(mail={login})))' -e DENDRONOTES_LDAP_USERNAME_ATTR=uid -e DENDRONOTES_LDAP_NAME_ATTR=cn -e DENDRONOTES_LDAP_ID_ATTR=entryUUID -e DENDRONOTES_LDAP_GROUP_FILTER='(&(objectClass=groupOfNames)(member={dn}))'
```
