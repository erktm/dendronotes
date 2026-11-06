# dendronotes

Self-hosted Markdown notes. Your notes stay plain `.md` files in a folder, the **soma**;
everything else – search index, sessions – is a derived cache that can be rebuilt at any time.

One container, one process, no external services, no external requests (air-gap friendly).

> **Status: milestone 3** – soma access, authentication, file tree, an editor with live
> preview and optional Vim key bindings, a reading view, synapses with backlinks, full-text
> search with tags, live updates when files change on disk, and synapses that follow renamed
> notes. Attachments follow in M4, Kubernetes manifests in M5.

## Vocabulary

| Term        | Meaning                                                                          |
|-------------|----------------------------------------------------------------------------------|
| **soma**    | The folder that holds your notes – the cell body everything grows from. Plural: somas. |
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

`make dev` creates a random password in `dev/password` on first start and prints it.

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
- **Renaming and moving.** Synapses in other notes are rewritten so they keep pointing to the
  renamed or moved note (Markdown links included), unless switched off in the settings. A note
  that was changed by someone else in that moment is left alone and reported.
- **Live updates.** Changes made outside dendronotes – another editor, git, a sync tool or a
  second browser tab – appear right away: the file tree, search and backlinks update, and an
  open note reloads. If it has unsaved edits, the comparison below opens instead.
- **Saving.** Notes save automatically while you type (⌘S / Ctrl+S saves at once). If the
  file was changed elsewhere in the meantime, nothing is overwritten: a comparison of both
  versions appears and you choose "Keep mine" or "Use version on disk".

| Shortcut         | Action                         |
|------------------|--------------------------------|
| ⌘O / Ctrl+O      | Quick switcher: open or create a note (Shift+Enter creates) |
| ⌘⇧F / Ctrl+Shift+F | Search notes                 |
| ⌘E / Ctrl+E      | Switch between edit and read   |
| ⌘S / Ctrl+S      | Save now                       |
| ⌘, / Ctrl+,      | Settings                       |

**Settings** (stored in the browser, never in the soma): live preview, Vim key bindings,
readable line length, note font (sans or serif), theme (system, light, dark), the details
panel, and whether renaming updates synapses.

Images and embedded files are shown as placeholders until attachments arrive in M4;
images from other hosts are never loaded.

## Index and live updates

dendronotes keeps a search index in `$DENDRONOTES_DATA_DIR/index.db` (SQLite with FTS5,
compiled into the binary, no extra service). It is a cache: delete it at any time and it is
rebuilt from the soma on the next start, as it is when its format changes or the file is
damaged. `/readyz` reports ready once the index has caught up with the soma.

Changes on disk are picked up through file system events (inotify on Linux). Every
`DENDRONOTES_RESCAN_INTERVAL` (default 5 minutes) the whole soma is compared as well, which
covers file systems without events, such as some network volumes. A very large soma may need
a higher inotify limit on the host (`fs.inotify.max_user_watches`); without it, changes are
still found by the rescans.

Browsers receive updates through Server-Sent Events (`/api/soma/events`). Behind a reverse
proxy, do not buffer that path and allow idle reads of at least 30 seconds (a heartbeat is
sent every 25 seconds); dendronotes sets `X-Accel-Buffering: no` for nginx.

## Running the container

```bash
docker run -d --name dendronotes -p 8080:8080 \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  -v /path/to/notes:/soma -v dendronotes-data:/data \
  -e DENDRONOTES_AUTH_PASSWORD_HASH='<output of hash-password>' \
  dendronotes:<version>
```

The image runs as UID 65532. `/soma` holds the notes, `/data` the rebuildable caches; both must
be writable. Back up the soma – nothing else is needed to restore everything.

## Configuration

All variables carry the `DENDRONOTES_` prefix. Variables without it are ignored.

| Variable                               | Default            | Meaning                                                    |
|----------------------------------------|--------------------|------------------------------------------------------------|
| `DENDRONOTES_SOMA_DIR`                 | `/soma`            | The soma: folder with the notes (source of truth)          |
| `DENDRONOTES_DATA_DIR`                 | `/data`            | Cache directory; must be writable and outside the soma     |
| `DENDRONOTES_HTTP_PORT`                | `8080`             | HTTP port                                                  |
| `DENDRONOTES_LOG_LEVEL`                | `info`             | `debug`, `info`, `warn`, `error`                           |
| `DENDRONOTES_LOG_FORMAT`               | `json`             | `json` (stdout) or `text`                                  |
| `DENDRONOTES_SHUTDOWN_TIMEOUT`         | `20s`              | Maximum time for in-flight requests after SIGTERM          |
| `DENDRONOTES_MAX_NOTE_MB`              | `5`                | Largest note the editor opens or saves                     |
| `DENDRONOTES_IGNORE`                   |                    | Extra name patterns hidden from the tree, e.g. `node_modules,*.tmp` (dot entries are always hidden) |
| `DENDRONOTES_RESCAN_INTERVAL`          | `5m`               | How often the whole soma is compared with the index, in addition to file system events; `0` turns it off |
| `DENDRONOTES_TRUSTED_PROXIES`          |                    | IPs/CIDRs of reverse proxies whose `X-Forwarded-*` headers are believed |
| `DENDRONOTES_AUTH_MODE`                | `password`         | `password` or `proxy`                                      |
| `DENDRONOTES_AUTH_PASSWORD_HASH`       |                    | argon2id hash (see below)                                  |
| `DENDRONOTES_AUTH_PASSWORD_HASH_FILE`  |                    | File containing the hash (e.g. a mounted secret)           |
| `DENDRONOTES_AUTH_PASSWORD_FILE`       |                    | File containing the plain password, hashed in memory at start |
| `DENDRONOTES_SESSION_TTL`              | `720h`             | Session lifetime, extended while in use                    |
| `DENDRONOTES_COOKIE_SECURE`            | `auto`             | `auto` (secure over HTTPS), `true` or `false`              |
| `DENDRONOTES_AUTH_PROXY_HEADER`        | `X-Forwarded-User` | Header carrying the user name in proxy mode                |
| `DENDRONOTES_AUTH_PROXY_ALLOWED_USERS` |                    | Optional comma-separated allow list for proxy mode         |
| `DENDRONOTES_MAX_UPLOAD_MB`            | `25`               | Largest file that can be uploaded (images, PDFs, media …)  |
| `DENDRONOTES_AUTH_PROXY_LOGOUT_URL`    |                    | Where "Sign out" leads in proxy mode, e.g. `/oauth2/sign_out` |

Exactly one of the three password variables is required in password mode; without
authentication configured, the server does not start.

The port variable is deliberately not called `DENDRONOTES_PORT`: Kubernetes injects
`DENDRONOTES_PORT` and `DENDRONOTES_SERVICE_*` into every pod when a Service named
`dendronotes` exists. On Kubernetes, also set `enableServiceLinks: false` in the pod spec.

Operational endpoints: `/healthz` (liveness), `/readyz` (readiness; 503 while starting,
while shutting down, or when the soma is unreachable).

## Authentication

**Single user with a password** (default). Create a hash and pass it as a secret:

```bash
docker run --rm -it dendronotes:<version> hash-password
```

Login attempts are rate-limited per client IP and globally; sessions are stored only as
SHA-256 hashes in `$DENDRONOTES_DATA_DIR/sessions.json` and sent as `HttpOnly`,
`SameSite=Strict` cookies (`__Host-` prefixed and `Secure` over HTTPS).

**Behind an authenticating proxy** (e.g. oauth2-proxy with Keycloak): set
`DENDRONOTES_AUTH_MODE=proxy`, `DENDRONOTES_TRUSTED_PROXIES` to the proxy's address range and
`DENDRONOTES_AUTH_PROXY_HEADER` to the header it sets (oauth2-proxy with
`--set-xauthrequest`: `X-Auth-Request-User`). The header is ignored from any other peer, so make
sure only the proxy can reach dendronotes (Kubernetes: a NetworkPolicy).

## Security notes

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
- The event stream carries only paths and versions, never note content, and ends when the
  session ends. Search snippets mark matches with control characters, not HTML.
- Note content is untrusted: the reading view renders it with markdown-it and sanitizes the
  result with DOMPurify (no scripts, event handlers, style attributes, forms, frames, SVG or
  `javascript:` URLs). External links open with `rel="noopener noreferrer"`. A test corpus of
  XSS payloads guards this.

## API

All endpoints return JSON. Everything that works on the soma lives below `/api/soma/`;
paths inside the soma are soma-relative and URL-encoded per segment. The full description
is in [`docs/openapi.yaml`](docs/openapi.yaml), which a test keeps in sync with the server.

| Method & path                           | Purpose                                              |
|-----------------------------------------|------------------------------------------------------|
| `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me` | Session handling         |
| `GET /api/soma/tree`                    | All visible files and folders                        |
| `GET /api/soma/notes/{path}`            | Read a note (with `ETag`)                            |
| `PUT /api/soma/notes/{path}`            | Save a note; requires `If-Match`, `412` on conflict  |
| `POST /api/soma/notes/{path}`           | Create a note                                        |
| `POST /api/soma/folders/{path}`         | Create a folder                                      |
| `POST /api/soma/move`                   | Rename or move: `{"from": …, "to": …, "updateSynapses": true}` |
| `DELETE /api/soma/entries/{path}`       | Move a file or folder to `.trash/` in the soma       |
| `GET /api/soma/search?q=`               | Full-text search (words, "phrases", #tags)           |
| `GET /api/soma/backlinks/{path}`        | Notes with synapses to a note                        |
| `GET /api/soma/tags`                    | All tags with their number of notes                  |
| `GET /api/soma/events`                  | Live updates (Server-Sent Events)                    |

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
internal/index/    SQLite cache: versions, synapses, tags, full-text search
internal/watch/    file system events (fsnotify) for changes made outside dendronotes
internal/events/   fan-out of live updates to connected browsers
internal/relink/   rewrites synapses when notes are renamed or moved
internal/auth/     password (argon2id), sessions, rate limit, proxy header
internal/clientip/ client address behind trusted proxies
internal/httpx/    JSON helpers
internal/server/   route table, middleware (CSP with nonce, logging, recovery), health, assets
web/               Svelte 5 frontend (Vite), embedded into the binary via go:embed
  src/lib/editor/  CodeMirror 6 setup, live preview, synapse syntax
  src/lib/         Markdown rendering + sanitizing, synapse resolution, state, UI strings
docs/openapi.yaml  API description
```

Contributors (human or AI): read [`CLAUDE.md`](CLAUDE.md) for the vocabulary and naming rules.
