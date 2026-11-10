# dendronotes

Self-hosted Markdown notes for one person or a team. Your notes stay plain `.md` files in
folders, the **somas**, readable and editable with any other tool; dendronotes adds editing
together in real time, synapses between notes with backlinks and a graph, full-text search,
drawings, comments, a history of every note, and sharing with people and groups.

One program, no database server, no external requests: it runs as a binary, a container or in
Kubernetes, also in an air gap.

## Features

- **Editor** with live preview, optional Vim key bindings and a reading view; tasks, tables,
  code, images, audio and video, embedded notes.
- **Synapses**: `[[Note]]`, `[[Note|Alias]]`, `[[Note#Heading]]` with suggestions; backlinks,
  a graph of all notes, synapses that follow renamed notes.
- **Editing together**: everyone in a note sees the others' changes and cursors at once;
  changes made on disk (Obsidian, git, a sync tool) are taken over live.
- **Drawings**: shapes, arrows that stick to shapes, text, freehand strokes and built-in icons
  for project management, software, cloud and networking; embedded in notes, downloadable as
  SVG or PNG.
- **Search** across all notes with phrases and `#tags`.
- **Comments** with `@mentions`, a **history** of every note, a **trash** with restore.
- **Accounts and sharing**: every account has its own somas, shared with people and groups in
  five roles; sign-in with passwords, Keycloak or another OpenID Connect provider, Active
  Directory / LDAP, or an authenticating proxy; personal API tokens.
- **Operation**: administration area, audit log, backups while running, Prometheus metrics,
  reproducible images, SBOM and vulnerability statements with every release.

## Vocabulary

| Term        | Meaning                                                                          |
|-------------|----------------------------------------------------------------------------------|
| **soma**    | A folder that holds notes. Plural: somas. Every account has its own.             |
| **note**    | A Markdown file in the soma.                                                     |
| **synapse** | A connection between two notes, e.g. the wiki link `[[Note]]`. Plural: synapses. |
| backlinks   | The synapses pointing *to* a note ("Incoming synapses").                         |

Files in a soma remain plain, Obsidian-compatible Markdown; the wiki link syntax is the usual
`[[Note]]`, `[[Note|Alias]]` and `[[Note#Heading]]`. Obsidian vaults can be opened as a soma.

## Getting started

The quickest way is the container:

```bash
mkdir -p somas && sudo chown 65532:65532 somas
head -c 18 /dev/urandom | base64 > admin-password && chmod 644 admin-password
docker run -d --name dendronotes -p 8080:8080 \
  -v "$PWD/somas:/somas" -v dendronotes-data:/data \
  -v "$PWD/admin-password:/run/secrets/admin-password:ro" \
  -e DENDRONOTES_ADMIN_PASSWORD_FILE=/run/secrets/admin-password \
  ghcr.io/erktm/dendronotes:latest
```

Open <http://localhost:8080> and sign in as `admin` with the password in `admin-password`.

| Way | Get it | Guide |
|-----|--------|-------|
| Binary (Linux, macOS; amd64, arm64) | [Releases](https://github.com/erktm/dendronotes/releases) | [installation → binary](docs/installation.md#binary) |
| Container | `ghcr.io/erktm/dendronotes` | [installation → container](docs/installation.md#container) |
| Kubernetes | `helm install dendronotes oci://ghcr.io/erktm/charts/dendronotes` | [installation → Helm](docs/installation.md#kubernetes-with-helm) |

## Documentation

- [User guide](docs/guide.md): notes, synapses, drawings, search, comments, history, sharing,
  administration
- [Installation and operation](docs/installation.md): binary, container, Helm, reverse proxy,
  backups, monitoring, upgrades, air gap
- [Authentication](docs/authentication.md): passwords, Keycloak / OpenID Connect, Active
  Directory / LDAP, authenticating proxy
- [Architecture](docs/architecture.md): parts, data, request flow, editing together, decisions
- [Security](docs/security.md): design, supply chain, SBOM and VEX; reporting in
  [SECURITY.md](SECURITY.md)
- [API](docs/api.md) and its description in [`docs/openapi.yaml`](docs/openapi.yaml)

## Configuration

All variables carry the `DENDRONOTES_` prefix. Variables without it are ignored.

| Variable                               | Default            | Meaning                                                    |
|----------------------------------------|--------------------|------------------------------------------------------------|
| `DENDRONOTES_SOMAS_DIR`                | `/somas`           | One folder per soma: the notes (source of truth); created if missing |
| `DENDRONOTES_SOMA_DIR`                 |                    | An existing soma folder, registered once as the first admin's soma (for upgrades) |
| `DENDRONOTES_DATA_DIR`                 | `/data`            | Account database `state.db` and search indexes; must be writable and outside the somas |
| `DENDRONOTES_HTTP_PORT`                | `8080`             | HTTP port                                                  |
| `DENDRONOTES_METRICS_PORT`             |                    | Port that serves only `/metrics` (Prometheus); empty: off. Keep it internal |
| `DENDRONOTES_LOG_LEVEL`                | `info`             | `debug`, `info`, `warn`, `error`                           |
| `DENDRONOTES_LOG_FORMAT`               | `json`             | `json` (stdout) or `text`                                  |
| `DENDRONOTES_SHUTDOWN_TIMEOUT`         | `20s`              | Maximum time for in-flight requests after SIGTERM          |
| `DENDRONOTES_MAX_NOTE_MB`              | `5`                | Largest note the editor opens or saves                     |
| `DENDRONOTES_MAX_UPLOAD_MB`            | `25`               | Largest file that can be uploaded (images, PDFs, media …)  |
| `DENDRONOTES_IGNORE`                   |                    | Extra name patterns hidden from the tree, e.g. `node_modules,*.tmp` (dot entries are always hidden) |
| `DENDRONOTES_RESCAN_INTERVAL`          | `5m`               | How often the whole soma is compared with the index, in addition to file system events; `0` turns it off |
| `DENDRONOTES_TRASH_DAYS`               | `7`                | Entries in a soma's trash are deleted for good after this many days (with their history and comments); `0` keeps them |
| `DENDRONOTES_HISTORY_DAYS`             | `90`               | How long earlier versions of notes are kept (the newest version of every note always stays); `0` keeps everything |
| `DENDRONOTES_TRUSTED_PROXIES`          |                    | IPs/CIDRs of reverse proxies whose `X-Forwarded-*` headers are believed |
| `DENDRONOTES_AUTH_MODE`                | `local`            | `local` (accounts with passwords) or `proxy`               |
| `DENDRONOTES_ADMIN_USERNAME`           | `admin`            | The first admin account, created at start while no admin exists |
| `DENDRONOTES_ADMIN_PASSWORD_HASH`      |                    | Its argon2id hash (see below), only used to create it      |
| `DENDRONOTES_ADMIN_PASSWORD_HASH_FILE` |                    | File containing that hash (e.g. a mounted secret)          |
| `DENDRONOTES_ADMIN_PASSWORD_FILE`      |                    | File containing the plain password, hashed at start        |
| `DENDRONOTES_SESSION_TTL`              | `720h`             | Session lifetime, extended while in use                    |
| `DENDRONOTES_COOKIE_SECURE`            | `auto`             | `auto` (secure over HTTPS), `true` or `false`              |
| `DENDRONOTES_AUTH_PROXY_HEADER`        | `X-Forwarded-User` | Header carrying the user name in proxy mode                |
| `DENDRONOTES_AUTH_PROXY_ALLOWED_USERS` |                    | Optional comma-separated allow list for proxy mode         |
| `DENDRONOTES_AUTH_PROXY_LOGOUT_URL`    |                    | Where "Sign out" leads in proxy mode, e.g. `/oauth2/sign_out` |
| `DENDRONOTES_PUBLIC_URL`               |                    | External address, e.g. `https://notes.example.org` (needed for OIDC) |
| `DENDRONOTES_LOCAL_LOGIN`              | `true`             | `false`: only the admin account may sign in with a local password (needs OIDC or LDAP) |
| `DENDRONOTES_OIDC_ISSUER`              |                    | Issuer URL of the provider, e.g. `https://sso.example.org/realms/main`; turns OIDC on |
| `DENDRONOTES_OIDC_CLIENT_ID`           |                    | Client ID registered for dendronotes                       |
| `DENDRONOTES_OIDC_CLIENT_SECRET_FILE`  |                    | File containing the client secret (none for a public client) |
| `DENDRONOTES_OIDC_SCOPES`              | `openid profile email` | Scopes to request (`openid` is always added)           |
| `DENDRONOTES_OIDC_USERNAME_CLAIM`      | `preferred_username` | Claim that becomes the user name                         |
| `DENDRONOTES_OIDC_GROUPS_CLAIM`        | `groups`           | Claim with the groups; dotted paths work, e.g. `realm_access.roles` |
| `DENDRONOTES_OIDC_ADMIN_GROUPS`        |                    | Comma-separated groups whose members are admins (synced on every sign-in) |
| `DENDRONOTES_OIDC_ALLOWED_GROUPS`      |                    | Comma-separated groups allowed to sign in (empty: everyone the provider lets through) |
| `DENDRONOTES_OIDC_LOGIN_LABEL`         | `SSO`              | Provider name on the sign-in button ("Continue with …")   |
| `DENDRONOTES_OIDC_CA_FILE`             |                    | PEM file with extra CA certificates for the provider (own CA, air gap) |
| `DENDRONOTES_LDAP_URL`                 |                    | Active Directory / LDAP server, `ldaps://dc1.example.org` (enables directory sign-in) |
| `DENDRONOTES_LDAP_START_TLS`           | `false`            | Encrypt an `ldap://` connection with StartTLS (plain `ldap://` works only for localhost) |
| `DENDRONOTES_LDAP_CA_FILE`             |                    | PEM file with extra CA certificates for the directory (own CA) |
| `DENDRONOTES_LDAP_BIND_DN`             |                    | Read-only service account that searches accounts and groups (empty: anonymous) |
| `DENDRONOTES_LDAP_BIND_PASSWORD_FILE`  |                    | File with that account's password                         |
| `DENDRONOTES_LDAP_BASE_DN`             |                    | Where accounts and groups are searched, e.g. `DC=example,DC=org` (required) |
| `DENDRONOTES_LDAP_USER_FILTER`         | AD: `sAMAccountName`, `userPrincipalName` or `mail` = `{login}` | Filter that finds the account; `{login}` is what was typed |
| `DENDRONOTES_LDAP_USERNAME_ATTR`       | `sAMAccountName`   | Attribute that becomes the user name                       |
| `DENDRONOTES_LDAP_NAME_ATTR`           | `displayName`      | Attribute with the display name                            |
| `DENDRONOTES_LDAP_EMAIL_ATTR`          | `mail`             | Attribute with the e-mail address                          |
| `DENDRONOTES_LDAP_ID_ATTR`             | `objectGUID`       | Stable id of accounts and groups (`entryUUID` for OpenLDAP) |
| `DENDRONOTES_LDAP_GROUP_BASE_DN`       |                    | Groups below this DN become dendronotes groups (empty: none) |
| `DENDRONOTES_LDAP_GROUP_FILTER`        | AD: groups with the account as (nested) `member` | Filter for an account's groups; `{dn}` is the account's DN |
| `DENDRONOTES_LDAP_GROUP_NAME_ATTR`     | `cn`               | Attribute with a group's name                              |
| `DENDRONOTES_LDAP_ADMIN_GROUPS`        |                    | Semicolon-separated groups (names or DNs) whose members are admins, synced on every sign-in |
| `DENDRONOTES_LDAP_ALLOWED_GROUPS`      |                    | Semicolon-separated groups (names or DNs) allowed to sign in (empty: every account found) |
| `DENDRONOTES_LDAP_SYNC_INTERVAL`       | `15m`              | How often all directory accounts are checked in the background (gone, disabled, groups); `0` turns it off |

In local mode, one of the three admin password variables is required while no enabled admin
account exists; without it, the server does not start. Once an admin exists, they are ignored
(the server says so in its log). Should no enabled admin be left, the next start with one of them
makes `DENDRONOTES_ADMIN_USERNAME` an enabled admin again with that password.

The port variable is deliberately not called `DENDRONOTES_PORT`: Kubernetes injects
`DENDRONOTES_PORT` and `DENDRONOTES_SERVICE_*` into every pod when a Service named
`dendronotes` exists. On Kubernetes, also set `enableServiceLinks: false` in the pod spec.

Operational endpoints: `/healthz` (liveness), `/readyz` (readiness; 503 while starting,
while shutting down, or when the account database or the somas directory is unreachable).

## Development

Requirements: Go (see `go.mod`), Node 26 with npm, Docker for the image, Helm for the chart.

```bash
make dev          # Go server :8080 + Vite dev server :5173 (somas in ./dev)
make test         # go vet, Go tests (-race), Svelte type check, frontend unit tests (Vitest)
make build        # bin/dendronotes with the embedded web interface
make image        # the container image (reproducible), loaded into Docker
make sbom         # security/sbom.cdx.json
make vulncheck    # govulncheck and npm audit against security/vex.openvex.json
make help         # all targets
```

`make dev` creates a random password for the account `admin` in `dev/password` on first start
and prints it.

## License

dendronotes is licensed under the [Apache License 2.0](LICENSE). The third-party components it
contains are listed in the [SBOM](security/sbom.cdx.json); their license texts are in
[THIRD_PARTY_LICENSES.txt](THIRD_PARTY_LICENSES.txt), which binaries and the image carry as well
(`/usr/share/doc/dendronotes/` in the image).
