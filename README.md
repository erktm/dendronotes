# dendronotes

Self-hosted Markdown notes. Your notes stay plain `.md` files in folders, the **somas**.
Accounts, who owns which soma and sessions live in a small SQLite database next to them; the
search indexes are caches that can be rebuilt at any time.

One container, one process, no external services, no external requests (air-gap friendly).

> **Status: milestone 9** – editing together in real time, accounts with their own somas,
> sharing with people and groups in five roles, sign-in through Keycloak (OpenID Connect) and
> an administration area, on top of the notes app: file tree, an editor with live preview and optional Vim key bindings, a
> reading view, synapses with backlinks, full-text search with tags, live updates when files
> change on disk, synapses that follow renamed notes, attachments (images, audio, video and
> other files) and embedded notes, and a graph of all notes and synapses.
> Next: Kubernetes manifests (M5).

## Vocabulary

| Term        | Meaning                                                                          |
|-------------|----------------------------------------------------------------------------------|
| **soma**    | A folder that holds notes – the cell body everything grows from. Plural: somas. Every account has its own. |
| **note**    | A Markdown file in the soma.                                                     |
| **synapse** | A connection between two notes, e.g. the wiki link `[[Note]]`. Plural: synapses. |
| backlinks   | The synapses pointing *to* a note ("Incoming synapses").                         |

Files in a soma remain plain, Obsidian-compatible Markdown; the wiki link syntax is the usual
`[[Note]]`, `[[Note|Alias]]` and `[[Note#Heading]]`. Obsidian vaults can be opened as a soma.

## Quick start

Requirements: Go ≥ 1.26, Node 26 / npm, Docker (for the image).

```bash
make dev          # Go server :8080 + Vite dev server :5173 (soma: ./dev/soma)
make test         # go vet, Go tests (-race), Svelte type check, frontend unit tests (Vitest)
make fuzz         # fuzz the path validation for 60 s
make build        # bin/dendronotes with the embedded frontend
make image        # build the container image and load it into Docker
make run-image    # run the image hardened (read-only, no capabilities)
```

`make dev` creates a random password for the account `admin` in `dev/password` on first start
and prints it. The dev server keeps its somas in `dev/somas` and registers `dev/soma` as the
admin's first soma.

## Writing notes

- **Edit and Read.** The editor renders Markdown as you type (live preview); the line with
  the cursor shows its syntax. The reading view shows the finished note. Switch with the
  toggle in the top bar or ⌘E / Ctrl+E.
- **Synapses.** Type `[[` for suggestions from the soma, and `[[Note#` for the headings of
  that note (`[[#` for this note's own). Click a synapse to open the note –
  in the editor, on the line you are editing, hold ⌘ / Ctrl while clicking. A synapse whose
  note does not exist yet is shown dashed; clicking it creates the note next to the current
  one (or at the path given, e.g. `[[Projects/Idea]]`). Markdown links to notes, such as
  `[Text](Other.md)`, are synapses too.
- **Tasks.** Checkboxes (`- [ ]`) can be ticked in both views; the file changes accordingly.
- **Details panel** (on wide screens). Outline of the note, its synapses, and the
  **incoming synapses**: every note that connects here, with the line it does so in.
- **Search** (⌘⇧F / Ctrl+Shift+F, or the magnifier in the left rail). Words match their
  beginning, `"quoted text"` an exact phrase and `#tag` a tag, including nested tags such as
  `#garden/beds`; case and accents are ignored. A result opens the note at the matching line.
  With an empty search field, the panel lists all tags.
- **Tags.** `#tag` anywhere in the text (not in code) or a `tags:` list in the front matter.
- **Attachments.** Paste an image or drop files into the editor: they are uploaded next to the
  note and embedded as `![[photo.png]]`. Files dropped on a folder in the tree (or picked with
  the upload button) go into that folder. Embeds show images (`![[photo.png|300]]` sets the
  width), audio and video players; other files, such as PDFs, become links that open them.
  Standard Markdown works too: `![alt](photo.png)`, `[report](report.pdf)`. Clicking a file in
  the tree shows it with its size and a download button. Images from other hosts are never
  loaded.
- **Graph** (⌘⇧G / Ctrl+Shift+G, or the graph button in the left rail). All notes as
  nodes, synapses as lines, sized by their number of synapses; unresolved synapses point to
  dashed nodes. Hover a note to see its neighborhood, click to open it, drag to arrange, scroll
  or use the buttons to zoom. "Around …" shows only the neighborhood of the note opened last,
  one to three steps deep; notes without synapses and unresolved synapses can be hidden.
- **Embedded notes.** `![[Note]]` shows another note inside this one in the reading view,
  `![[Note#Heading]]` only that section (up to three levels deep).
- **Renaming and moving.** Synapses in other notes are rewritten so they keep pointing to the
  renamed or moved note (Markdown links included), and so are embeds and links to renamed or
  moved attachments (`![[photo.png]]`, `![](photo.png)`, `[report](report.pdf)`), unless
  switched off in the settings. A note that was changed by someone else in that moment is left
  alone and reported.
- **Editing together.** Everyone who has a note open – other people the soma is shared with,
  or your own second tab – edits the same text at once: changes appear right away, the others'
  cursors and selections show with their names, and the bar at the bottom of the note shows who
  else is in it. Readers follow along in the reading view without being able to change anything.
- **Saving.** The server writes a note about a second after typing stops, at the latest every
  five seconds while typing goes on ("Saving …", then "Saved"); ⌘S / Ctrl+S saves at once.
  Files keep their line breaks (also Windows-style) and a byte order mark. Without a
  connection, changes stay in the tab ("Offline – changes kept here") and are sent as soon as
  it is back – also if the server restarted meanwhile: they are merged into the current text.
- **History** (the clock in the top bar). Every save is kept as a version: saves by the same
  people within ten minutes count as one, changes made outside dendronotes and restored
  versions get their own. The history lists who changed what and when, shows the changes from
  any version to the current note line by line, and – for everyone who may change notes –
  brings a version back (the current text stays in the history). It follows renames and the
  trash; versions older than `DENDRONOTES_HISTORY_DAYS` (90 days) are removed, but the newest
  version of every note always stays. The history lives in `DENDRONOTES_DATA_DIR/history/`,
  never in the soma.
- **Comments** (the speech bubble in the top bar). Select text – in the editor or the reading
  view – and press "Comment", or comment on the whole note. Everyone with access comments and
  replies, readers too; writers mark threads as done, authors change or delete their own
  comments, editors anyone's. The commented text is marked in the note and found again after
  edits; if it is gone, the thread says so. `@user name` lets someone with access know, and
  replies notify the others in the thread: the bell in the top bar lists both and jumps to
  the comment. Comments live in `state.db`, never in the note, and follow renames and the
  trash.
- **Trash** (the bin at the bottom of the sidebar, for everyone who may delete). Deleted files
  and folders wait in the soma's `.trash` folder – the same place Obsidian uses – with who
  deleted them, when and from where. Restore puts them back where they were (missing folders
  are created again, an occupied name gets a " (n)" suffix); the owner can also delete single
  entries or everything for good, which takes their history along.
- **Download as ZIP** (soma switcher). All visible notes, files and folders of the soma as they
  are now; hidden folders such as `.trash` and `.obsidian` stay out.
- **Live updates.** Changes made outside dendronotes – another editor, git, a sync tool –
  appear right away: the file tree, search and backlinks update, and an open note takes over
  the change. If it was edited here at the same time, both are merged line by line; where both
  sides changed the same lines, both versions are kept one after the other (and a notice says
  so) – nothing is lost.

| Shortcut         | Action                         |
|------------------|--------------------------------|
| ⌘O / Ctrl+O      | Quick switcher: open or create a note (Shift+Enter creates) |
| ⌘⇧F / Ctrl+Shift+F | Search notes                 |
| ⌘⇧G / Ctrl+Shift+G | Graph (again: back to the note) |
| ⌘E / Ctrl+E      | Switch between edit and read   |
| ⌘S / Ctrl+S      | Save now                       |
| ⌘, / Ctrl+,      | Settings                       |

**Settings** (stored in the browser, never in the soma): live preview, Vim key bindings,
readable line length, note font (sans or serif), theme (system, light, dark), color palette,
the details panel, and whether renaming updates synapses.

**Color palettes**, each with a light and a dark variant: Cortex (the default; gray matter,
graphite with a touch of rose), Sage (leaf green) and six taken from the ways nerve tissue is
made visible – the stains Golgi (ink on aged paper, silver turned gold), Nissl (cresyl violet),
Eosin (H&E: pink and blue-violet) and Myelin (Luxol fast blue with a rose counterstain), and the
fluorescence of Confocal (cyan and magenta) and Brainbow (coral and turquoise).

Uploads are limited to images (PNG, JPEG, GIF, WebP, AVIF, BMP, ICO, SVG), PDF, audio (MP3,
WAV, Ogg, FLAC, M4A), video (MP4, WebM, MOV), text (Markdown, TXT, CSV, JSON) and zip files,
up to `DENDRONOTES_MAX_UPLOAD_MB`. The content must match the extension.

## Accounts, somas and administration

- **Somas.** Every account has its own somas; new accounts start with an empty one called
  "Notes". The switcher at the top of the sidebar changes the soma ("Switch soma") and creates,
  renames or deletes one. Each soma is a folder in `DENDRONOTES_SOMAS_DIR`, named after the
  soma when it is created (renaming the soma keeps the folder). Deleting a soma moves its
  folder to `DENDRONOTES_SOMAS_DIR/.trash/`; admins find it under *Somas → Deleted somas* and
  restore it with its notes and shares, or delete it for good. A soma registered from
  `DENDRONOTES_SOMA_DIR` keeps its folder where it is.
- **Sharing** ("Share soma …" in the soma switcher). The owner adds people and groups by name,
  each with a role; every role includes the ones above it:

  | Role | May |
  |------|-----|
  | *Can read* (viewer) | read notes and files |
  | *Can write* (writer) | change existing notes and tick tasks |
  | *Can add* (contributor) | create notes, folders and files (uploads, new notes from synapses) |
  | *Full access* (editor) | rename, move and delete |
  | *Owner* | share, rename or delete the soma, hand it over ("Make owner"; the previous owner keeps full access) |

  Someone who gets a soma both personally and through groups has the highest of those roles.
  Somas shared with you are listed under "Shared with you"; "People with access …" shows who
  else is in and lets you leave a soma shared with you personally (access through a group
  stays). What a role does not allow is hidden in the interface and refused by the server.
  Changes to sharing take effect at once, also in browsers that have the soma open – someone
  who loses access is disconnected right away.
- **Profile** (your picture at the top right, which also signs you out): profile picture,
  display name, e-mail and, for accounts with a password, a new password. Changing it signs
  out your other sessions.
- **Profile pictures**: design one from a color, a pattern (dendrite, myelin rings, vesicles)
  and your initials, or upload a photo and place it in a circle. The browser crops the photo to
  a small square; the server stores it in `state.db`, encoded anew without metadata, and shows
  it only to people you share a soma with (and to admins). Pictures appear in the header (yours),
  at the bottom of a note (the others in it), in the sharing dialog and in the administration.
- **Administration** (the shield in the left rail, admins only):
  - *Users*: create accounts, change role, disable, set a new password, sign out everywhere,
    delete. A deleted account's somas go to an account you choose or to the trash. The last
    admin can be neither demoted, disabled nor deleted.
  - *Groups*: create, rename and delete groups and choose their members. The group
    *Everyone* is built in and contains every account, also future ones; share a soma with it
    to open it to the whole server. Groups taken over from Active Directory are marked as such;
    their name and members come from the directory (see [Authentication](#authentication)).
  - *Somas*: every soma with owner, members, groups, size and folder; create a soma for any
    account – optionally shared with a group right away, for example *Everyone* –, hand a soma
    to another owner or delete it.
  - *Sessions*: who is signed in from where; end single sessions.
  - *Server*: version, uptime, storage, open somas, a backup download, whether the OpenID
    Connect provider and the directory are reachable, and the effective configuration
    (secrets show only whether they are set).
  - *Audit log*: sign-ins (failed ones too), account and soma changes.

Disabling an account, setting its password or ending its sessions takes effect at once; open
browser tabs notice within 25 seconds.

## Index and live updates

dendronotes keeps a search index per soma in `$DENDRONOTES_DATA_DIR/index/<soma id>/index.db`
(SQLite with FTS5, compiled into the binary, no extra service). It is a cache: delete it at
any time and it is rebuilt from the soma the next time the soma is opened, as it is when its
format changes or the file is damaged. Somas are opened on first use and closed after 30
minutes without connected browsers.

Changes on disk are picked up through file system events (inotify on Linux). Every
`DENDRONOTES_RESCAN_INTERVAL` (default 5 minutes) the whole soma is compared as well, which
covers file systems without events, such as some network volumes. A very large soma may need
a higher inotify limit on the host (`fs.inotify.max_user_watches`); without it, changes are
still found by the rescans.

Browsers receive updates through Server-Sent Events (`/api/somas/{soma}/events`). Behind a reverse
proxy, do not buffer that path and allow idle reads of at least 30 seconds (a heartbeat is
sent every 25 seconds); dendronotes sets `X-Accel-Buffering: no` for nginx.

## Running the container

```bash
docker run -d --name dendronotes -p 8080:8080 \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  -v /path/to/somas:/somas -v dendronotes-data:/data \
  -e DENDRONOTES_ADMIN_PASSWORD_HASH='<output of hash-password>' \
  dendronotes:<version>
```

The image runs as UID 65532. `/somas` holds the notes (one folder per soma), `/data` the
account database `state.db` and the search indexes; both must be writable.

**Backups** need both: the somas directory and what in `/data` is not a cache – `state.db`
(accounts, shares, groups, profile pictures, the trash records) and the note histories in
`/data/history/`. The indexes in `/data/index` are caches and can be left out. The databases
are written while running, so copy them consistently: *Admin → Server → Download backup*, or

```bash
docker exec dendronotes dendronotes backup /data/backup-$(date +%F).zip
```

Both make a ZIP of consistent copies while dendronotes keeps running; to restore, stop it and
unpack the ZIP into `/data`. The somas directory is plain files: back it up as such (or use
volume snapshots for everything). Before a new release changes the database format,
dendronotes copies `state.db` to `state.db.v<n>.bak`; it refuses to open a database written by
a newer release.

**Monitoring.** With `DENDRONOTES_METRICS_PORT` (e.g. `9090`) dendronotes serves Prometheus
metrics on that port – only `/metrics`, nothing else, so keep the port internal (a separate
container port, not routed through the ingress): requests by route pattern, method and status
with durations, sign-ins by way and result, accounts, sessions, somas, open somas, live
connections, notes being edited, disk usage, whether the OpenID Connect provider and the
directory answer, the last directory sync, and Go runtime figures. Note paths never become
labels.

**Locked out?** `dendronotes reset-password [user]` sets a new password for a local account
(by default the admin account), enables it and ends its sessions – also while the server runs:

```bash
docker exec -it dendronotes dendronotes reset-password
```

**Upgrading from a single soma** (before M6): mount the old notes folder, e.g. at `/soma`, and
set `DENDRONOTES_SOMA_DIR=/soma`. On the first start with no somas registered, it becomes the
admin's soma "Notes" and stays in place.

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
| `DENDRONOTES_HISTORY_DAYS`             | `90`               | How long earlier versions of notes are kept (the newest version of every note always stays); `0` keeps everything |
| `DENDRONOTES_TRUSTED_PROXIES`          |                    | IPs/CIDRs of reverse proxies whose `X-Forwarded-*` headers are believed |
| `DENDRONOTES_AUTH_MODE`                | `local`            | `local` (accounts with passwords) or `proxy`               |
| `DENDRONOTES_ADMIN_USERNAME`           | `admin`            | The first admin account, created at start while no admin exists |
| `DENDRONOTES_ADMIN_PASSWORD_HASH`      |                    | Its argon2id hash (see below) – only used to create it     |
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

## Authentication

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
  secret into a file for `DENDRONOTES_OIDC_CLIENT_SECRET_FILE`) or off for a public client –
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
sign-in – to end it at once, disable the account or end its sessions in the administration area.

With `DENDRONOTES_LOCAL_LOGIN=false` the sign-in page shows only the Keycloak button; the admin
account can still sign in with its password at `/?local=1` should the provider be unreachable.
The administration area shows whether the provider is reachable.

**With Active Directory** (or another LDAP directory; in local mode, next to or instead of
passwords). People sign in on the normal form with their directory account – user name
(`sAMAccountName`), `user@domain` (UPN) or e-mail – and its password. dendronotes looks the
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
- **Groups**: every group below `DENDRONOTES_LDAP_GROUP_BASE_DN` that someone is a member of –
  directly or through nested groups – becomes a dendronotes group at their sign-in, so somas can
  be shared with it like with any group. Name and members follow the directory: someone joins
  or leaves a group at their next sign-in. In the administration area these groups are marked
  *Active Directory*; they can be deleted (the next member's sign-in brings them back) but not
  renamed or given other members. Keep the group base narrow – a dedicated OU – rather than the
  whole domain. A local group with the same name stays separate; the directory group is shown
  as "Name (AD)".
- **Sync in the background** (`DENDRONOTES_LDAP_SYNC_INTERVAL`, every 15 minutes; `0` turns it
  off): every directory account is looked up again by its `objectGUID`. Accounts that are gone,
  disabled in Active Directory or no longer in `LDAP_ALLOWED_GROUPS` are signed out everywhere
  and lose their API tokens – the account itself stays, its next sign-in fails. The others take
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

## Security notes

- Somas are separated per account: requests for a soma the account has no access to (neither
  personally nor through a group) are answered with 404, as if it did not exist; members with
  too low a role get 403. The administration needs the admin role. A test runs every route as
  non-member and with every role, given personally and through a group.
- OpenID Connect uses the authorization code flow with PKCE (S256), a random state bound to
  the browser by a short-lived cookie (no login CSRF, each state works once) and a nonce; the
  ID token's signature, issuer, audience, expiry and nonce are checked. After sign-in the
  browser only returns to paths within dendronotes. The search for people to share with needs
  two characters and shows only user and display names (and group names).
- Every file access goes through Go's `os.Root`: paths with `..`, absolute paths and symlinks
  pointing out of the soma are refused by the kernel-level lookup. API paths are validated
  beforehand and can never address hidden entries (`.git`, `.trash`, …). Symlinks inside the
  soma are shown read-only.
- Saving requires the version the client started from (`If-Match`); changes made elsewhere in
  the meantime are never overwritten silently.
- State-changing requests from other origins are rejected (`Sec-Fetch-Site`/`Origin`); the API
  only accepts JSON bodies.
- Strict Content-Security-Policy with per-response nonce; no inline scripts, no `eval`, no
  external hosts. The only inline style allowed is the exact `tab-size: 4` attribute the
  editor sets (by hash).
- Uploads must have an allowed extension and content that matches it (a `.png` must be a
  PNG); HTML, scripts and executables are refused. Files are served with the type of their
  extension, `nosniff`, and a sandboxing Content-Security-Policy, so a file opened directly in
  the browser (an SVG, say) can never run scripts. Notes show media only from the soma.
- `change` events on the event stream carry only paths and versions. Note content travels
  only to the tabs that joined that note for editing together, which needs read access to the
  soma; pushing changes needs edit access. The server checks every change set (structure,
  length against the document, size limit) before applying it, and limits how many updates
  and cursor moves a tab may send. Streams end when the session ends or the account loses
  access to the soma. Search snippets mark matches with control characters, not HTML.
- Note content is untrusted: the reading view renders it with markdown-it and sanitizes the
  result with DOMPurify (no scripts, event handlers, style attributes, forms, frames, SVG or
  `javascript:` URLs). External links open with `rel="noopener noreferrer"`. A test corpus of
  XSS payloads guards this.

## API

All endpoints return JSON. Everything that works on a soma lives below
`/api/somas/{soma}/` (`{soma}` is the soma's id); paths inside the soma are soma-relative and
URL-encoded per segment. The full description is in [`docs/openapi.yaml`](docs/openapi.yaml),
which a test keeps in sync with the server.

| Method & path                           | Purpose                                              |
|-----------------------------------------|------------------------------------------------------|
| `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me` | Session handling         |
| `PATCH /api/me`, `POST /api/me/password` | Own profile and password                            |
| `PUT /api/me/avatar`, `PUT`/`DELETE /api/me/avatar/photo`, `GET /api/users/{id}/avatar` | Profile picture: design, photo |
| `GET /api/somas`, `POST /api/somas`     | The account's somas; create one                      |
| `PATCH /api/somas/{soma}`, `DELETE /api/somas/{soma}` | Rename, hand over or delete a soma (owner) |
| `GET /api/somas/{soma}/tree`            | All visible files and folders                        |
| `GET /api/somas/{soma}/notes/{path}`    | Read a note (with `ETag`)                            |
| `PUT /api/somas/{soma}/notes/{path}`    | Save a note; requires `If-Match`, `412` on conflict  |
| `POST /api/somas/{soma}/notes/{path}`   | Create a note                                        |
| `POST /api/somas/{soma}/folders/{path}` | Create a folder                                      |
| `POST /api/somas/{soma}/move`           | Rename or move: `{"from": …, "to": …, "updateSynapses": true}` |
| `DELETE /api/somas/{soma}/entries/{path}` | Move a file or folder to `.trash/` in the soma     |
| `GET /api/somas/{soma}/trash`           | The trash; `POST …/trash/{path}` restores, `DELETE` deletes for good (owner) |
| `GET /api/somas/{soma}/history/{path}`  | Versions of a note; `GET …/versions/{id}`, `POST …/versions/{id}/restore` |
| `GET /api/somas/{soma}/export`          | The soma as a ZIP                                    |
| `GET /api/somas/{soma}/comments?path=`  | Comments on a note; `POST` adds one (or a reply), `PATCH`/`DELETE …/comments/{id}`, `POST …/{id}/resolve` |
| `GET /api/me/notifications`             | Mentions and replies; `POST …/read` marks them read  |
| `GET /api/somas/{soma}/files/{path}`    | Download a file (attachments, images, media)         |
| `POST /api/somas/{soma}/files/{path}`   | Upload a file (raw body; `?unique=1` avoids name clashes) |
| `GET /api/somas/{soma}/search?q=`       | Full-text search (words, "phrases", #tags)           |
| `GET /api/somas/{soma}/backlinks/{path}` | Notes with synapses to a note                       |
| `GET /api/somas/{soma}/tags`            | All tags with their number of notes                  |
| `GET /api/somas/{soma}/graph`           | All notes and the synapses between them              |
| `GET /api/somas/{soma}/events`          | Live updates (Server-Sent Events)                    |
| `POST /api/somas/{soma}/collab/join`, `/leave`, `/push`, `/presence`; `GET …/collab/pull` | Editing together: open a note with the stream's id, send changes (CodeMirror change sets), cursors, catch up |
| `GET /api/somas/{soma}/members`         | Who has access – people and groups – with roles     |
| `PUT`, `DELETE /api/somas/{soma}/members/{id}` | Share with an account or change its role; take access away (owner) |
| `PUT`, `DELETE /api/somas/{soma}/groups/{id}` | Share with a group or change its role; take access away (owner) |
| `DELETE /api/somas/{soma}/membership`   | Leave a soma shared with you personally              |
| `GET /api/users/search?q=`              | Find people to share with (two characters or more)   |
| `GET /api/auth/methods`, `/api/auth/oidc/login`, `/api/auth/oidc/callback` | Sign-in options and the OpenID Connect flow |
| `/api/admin/users`, `/groups`, `/somas`, `/sessions`, `/server`, `/sso/check`, `/audit` | Administration (admins only) |

**Personal API tokens** (profile → *API tokens*) let scripts use the API as their account:
"Read" tokens only read, "Read and write" tokens also change notes and files. No token manages
anything – administration, sharing, renaming or deleting somas, the account and its tokens
answer `403 token_not_allowed`. A token is shown once; only its hash is stored.

```bash
curl -H "Authorization: Bearer dn_…" https://notes.example.org/api/somas
```

## Air gap and reproducible builds

```bash
make vendor          # vendor/ (Go) + .offline/npm-cache (npm, Linux binaries for amd64/arm64)
make image-offline   # rebuild without cache and without network in the build steps
```

- Base images are pinned by digest in the `Dockerfile`. In an air gap, mirror them into a local
  registry and pass `--build-arg NODE_IMAGE=… GO_IMAGE=… RUNTIME_IMAGE=…`.
- `make image` also writes `bin/dendronotes-<version>-<platform>.oci.tar` for transfer.
- Same source ⇒ same image digest (`SOURCE_DATE_EPOCH`, `rewrite-timestamp`, no provenance
  attestation with timestamps).
- `npm run build` fails if the bundle contains unexpected external URLs.

## Layout

```
cmd/dendronotes/   entry point: serve, hash-password, reset-password, backup, healthcheck, version
internal/config/   read and validate DENDRONOTES_* environment variables
internal/soma/     soma access exclusively through os.Root (tree, notes, move, trash)
internal/mdparse/  synapses and tags of a note (same rules as web/src/lib/synapses.ts)
internal/store/    state.db: accounts, sessions, avatars, somas and members, audit log (versioned migrations)
internal/avatar/   checks uploaded profile photos and encodes them anew (square PNG, no metadata)
internal/somas/    soma manager: opens somas on demand (index, watcher, live updates), closes idle ones
internal/collab/   editing together: live documents, change sets (UTF-16), saving, 3-way merge
internal/index/    SQLite cache per soma: versions, synapses, tags, full-text search
internal/history/  earlier versions of notes, one SQLite database per soma (not a cache)
internal/backup/   consistent copies of state.db and the histories as a ZIP
internal/watch/    file system events (fsnotify) for changes made outside dendronotes
internal/events/   fan-out of live updates to connected browsers
internal/relink/   rewrites synapses when notes are renamed or moved
internal/attach/   which files may be uploaded (type allow list, content check) and how they are served
internal/auth/     sign-in (argon2id, OIDC accounts, proxy header), sessions, rate limits, first admin
internal/sso/      OpenID Connect client: discovery, PKCE, ID token checks, groups
internal/ldap/     Active Directory / LDAP: service-account search, bind, nested groups
internal/clientip/ client address behind trusted proxies
internal/httpx/    JSON helpers
internal/server/   route table with access levels, API and admin handlers, middleware (CSP with nonce,
                   logging, recovery), health, assets
web/               Svelte 5 frontend (Vite), embedded into the binary via go:embed
  src/lib/editor/  CodeMirror 6 setup, live preview, synapse syntax
  src/lib/         Markdown rendering + sanitizing, synapse resolution, state, UI strings
docs/openapi.yaml  API description
```

Contributors (human or AI): read [`CLAUDE.md`](CLAUDE.md) for the vocabulary and naming rules.
