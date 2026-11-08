# dendronotes

Self-hosted Markdown notes. Your notes stay plain `.md` files in folders, the **somas**.
Accounts, who owns which soma and sessions live in a small SQLite database next to them; the
search indexes are caches that can be rebuilt at any time.

One container, one process, no external services, no external requests (air-gap friendly).

> **Status: milestone 8** – editing together in real time, accounts with their own somas,
> sharing with roles, sign-in through Keycloak (OpenID Connect) and an administration area, on
> top of the notes app: file tree, an editor with live preview and optional Vim key bindings, a
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
  renamed or moved note (Markdown links included), unless switched off in the settings. A note
  that was changed by someone else in that moment is left alone and reported.
- **Editing together.** Everyone who has a note open – other people the soma is shared with,
  or your own second tab – edits the same text at once: changes appear right away, the others'
  cursors and selections show with their names, and their initials appear at the top. Readers
  follow along without being able to change anything.
- **Saving.** The server writes a note about a second after typing stops, at the latest every
  five seconds while typing goes on ("Saving …", then "Saved"); ⌘S / Ctrl+S saves at once.
  Files keep their line breaks (also Windows-style) and a byte order mark. Without a
  connection, changes stay in the tab ("Offline – changes kept here") and are sent as soon as
  it is back – also if the server restarted meanwhile: they are merged into the current text.
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
  folder to `DENDRONOTES_SOMAS_DIR/.trash/`, so the notes stay recoverable as files; a soma
  registered from `DENDRONOTES_SOMA_DIR` keeps its folder where it is.
- **Sharing** ("Share soma …" in the soma switcher). The owner adds people by name with a
  role: *can edit* (notes and files) or *can read*. Only the owner shares, renames, deletes or
  hands the soma over to another member ("Make owner"; the previous owner stays as editor).
  Somas shared with you are listed under "Shared with you"; "People with access …" shows who
  else is in and lets you leave. For readers the soma is read-only: no editing, uploads,
  renaming or deleting – the server refuses such requests as well. Changes to sharing take
  effect at once, also in browsers that have the soma open.
- **Profile** (click your name at the bottom of the sidebar): display name, e-mail and, for
  accounts with a password, a new password. Changing it signs out your other sessions.
- **Administration** (the shield in the left rail, admins only):
  - *Users*: create accounts, change role, disable, set a new password, sign out everywhere,
    delete. A deleted account's somas go to an account you choose or to the trash. The last
    admin can be neither demoted, disabled nor deleted.
  - *Somas*: every soma with owner, size and folder; hand it to another owner or delete it.
  - *Sessions*: who is signed in from where; end single sessions.
  - *Server*: version, uptime, storage, open somas and the effective configuration (secrets
    show only whether they are set).
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

**Backups** need both: the somas directory and `state.db` (accounts, sessions, who owns which
soma). Stop the container for the backup or use a volume snapshot, since `state.db` is written
while running. The indexes in `/data/index` are caches and can be left out. Before a new
release changes the database format, dendronotes copies it to `state.db.v<n>.bak`; it refuses
to open a database written by a newer release.

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
| `DENDRONOTES_LOG_LEVEL`                | `info`             | `debug`, `info`, `warn`, `error`                           |
| `DENDRONOTES_LOG_FORMAT`               | `json`             | `json` (stdout) or `text`                                  |
| `DENDRONOTES_SHUTDOWN_TIMEOUT`         | `20s`              | Maximum time for in-flight requests after SIGTERM          |
| `DENDRONOTES_MAX_NOTE_MB`              | `5`                | Largest note the editor opens or saves                     |
| `DENDRONOTES_MAX_UPLOAD_MB`            | `25`               | Largest file that can be uploaded (images, PDFs, media …)  |
| `DENDRONOTES_IGNORE`                   |                    | Extra name patterns hidden from the tree, e.g. `node_modules,*.tmp` (dot entries are always hidden) |
| `DENDRONOTES_RESCAN_INTERVAL`          | `5m`               | How often the whole soma is compared with the index, in addition to file system events; `0` turns it off |
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
| `DENDRONOTES_LOCAL_LOGIN`              | `true`             | `false`: only the admin account may sign in with a password (needs OIDC) |
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
name and password (at least 10 characters) and can change the password in their profile.
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

## Security notes

- Somas are separated per account: requests for a soma the account is not a member of are
  answered with 404, as if it did not exist; members with too low a role get 403 (readers on
  every changing request). The administration needs the admin role. A test runs every route
  as non-member, reader and editor.
- OpenID Connect uses the authorization code flow with PKCE (S256), a random state bound to
  the browser by a short-lived cookie (no login CSRF, each state works once) and a nonce; the
  ID token's signature, issuer, audience, expiry and nonce are checked. After sign-in the
  browser only returns to paths within dendronotes. The search for people to share with needs
  two characters and shows only user and display names.
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
| `GET /api/somas`, `POST /api/somas`     | The account's somas; create one                      |
| `PATCH /api/somas/{soma}`, `DELETE /api/somas/{soma}` | Rename, hand over or delete a soma (owner) |
| `GET /api/somas/{soma}/tree`            | All visible files and folders                        |
| `GET /api/somas/{soma}/notes/{path}`    | Read a note (with `ETag`)                            |
| `PUT /api/somas/{soma}/notes/{path}`    | Save a note; requires `If-Match`, `412` on conflict  |
| `POST /api/somas/{soma}/notes/{path}`   | Create a note                                        |
| `POST /api/somas/{soma}/folders/{path}` | Create a folder                                      |
| `POST /api/somas/{soma}/move`           | Rename or move: `{"from": …, "to": …, "updateSynapses": true}` |
| `DELETE /api/somas/{soma}/entries/{path}` | Move a file or folder to `.trash/` in the soma     |
| `GET /api/somas/{soma}/files/{path}`    | Download a file (attachments, images, media)         |
| `POST /api/somas/{soma}/files/{path}`   | Upload a file (raw body; `?unique=1` avoids name clashes) |
| `GET /api/somas/{soma}/search?q=`       | Full-text search (words, "phrases", #tags)           |
| `GET /api/somas/{soma}/backlinks/{path}` | Notes with synapses to a note                       |
| `GET /api/somas/{soma}/tags`            | All tags with their number of notes                  |
| `GET /api/somas/{soma}/graph`           | All notes and the synapses between them              |
| `GET /api/somas/{soma}/events`          | Live updates (Server-Sent Events)                    |
| `POST /api/somas/{soma}/collab/join`, `/leave`, `/push`, `/presence`; `GET …/collab/pull` | Editing together: open a note with the stream's id, send changes (CodeMirror change sets), cursors, catch up |
| `GET /api/somas/{soma}/members`         | Who has access, with roles                           |
| `PUT`, `DELETE /api/somas/{soma}/members/{id}` | Share with an account or change its role; take access away (owner) |
| `DELETE /api/somas/{soma}/membership`   | Leave a soma shared with you                         |
| `GET /api/users/search?q=`              | Find people to share with (two characters or more)   |
| `GET /api/auth/methods`, `/api/auth/oidc/login`, `/api/auth/oidc/callback` | Sign-in options and the OpenID Connect flow |
| `/api/admin/users`, `/somas`, `/sessions`, `/server`, `/sso/check`, `/audit` | Administration (admins only) |

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
cmd/dendronotes/   entry point: serve, hash-password, healthcheck, version
internal/config/   read and validate DENDRONOTES_* environment variables
internal/soma/     soma access exclusively through os.Root (tree, notes, move, trash)
internal/mdparse/  synapses and tags of a note (same rules as web/src/lib/synapses.ts)
internal/store/    state.db: accounts, sessions, somas and members, audit log (versioned migrations)
internal/somas/    soma manager: opens somas on demand (index, watcher, live updates), closes idle ones
internal/collab/   editing together: live documents, change sets (UTF-16), saving, 3-way merge
internal/index/    SQLite cache per soma: versions, synapses, tags, full-text search
internal/watch/    file system events (fsnotify) for changes made outside dendronotes
internal/events/   fan-out of live updates to connected browsers
internal/relink/   rewrites synapses when notes are renamed or moved
internal/attach/   which files may be uploaded (type allow list, content check) and how they are served
internal/auth/     sign-in (argon2id, OIDC accounts, proxy header), sessions, rate limits, first admin
internal/sso/      OpenID Connect client: discovery, PKCE, ID token checks, groups
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
