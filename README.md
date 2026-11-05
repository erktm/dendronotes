# leafnotes

Self-hosted Markdown notes. Your notes stay plain `.md` files in a folder (the *vault*);
everything else – search index, sessions – is a derived cache that can be rebuilt at any time.

One container, one process, no external services, no external requests (air-gap friendly).

> **Status: milestone 1** – vault access, authentication, file tree (create, rename, move by
> drag & drop, delete to `.trash`) and a plain editor with autosave and conflict detection.
> Live preview, search, backlinks and live sync follow in M2–M5.

## Quick start

Requirements: Go ≥ 1.26, Node 26 / npm, Docker (for the image).

```bash
make dev          # Go server :8080 + Vite dev server :5173 (vault: ./dev/vault)
make test         # go vet, Go tests (-race), Svelte type check
make fuzz         # fuzz the path validation for 60 s
make build        # bin/leafnotes with the embedded frontend
make image        # build the container image and load it into Docker
make run-image    # run the image hardened (read-only, no capabilities)
```

`make dev` creates a random password in `dev/password` on first start and prints it.

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
| `DENDRONOTES_TRUSTED_PROXIES`          |                    | IPs/CIDRs of reverse proxies whose `X-Forwarded-*` headers are believed |
| `DENDRONOTES_AUTH_MODE`                | `password`         | `password` or `proxy`                                      |
| `DENDRONOTES_AUTH_PASSWORD_HASH`       |                    | argon2id hash (see below)                                  |
| `DENDRONOTES_AUTH_PASSWORD_HASH_FILE`  |                    | File containing the hash (e.g. a mounted secret)           |
| `DENDRONOTES_AUTH_PASSWORD_FILE`       |                    | File containing the plain password, hashed in memory at start |
| `DENDRONOTES_SESSION_TTL`              | `720h`             | Session lifetime, extended while in use                    |
| `DENDRONOTES_COOKIE_SECURE`            | `auto`             | `auto` (secure over HTTPS), `true` or `false`              |
| `DENDRONOTES_AUTH_PROXY_HEADER`        | `X-Forwarded-User` | Header carrying the user name in proxy mode                |
| `DENDRONOTES_AUTH_PROXY_ALLOWED_USERS` |                    | Optional comma-separated allow list for proxy mode         |
| `DENDRONOTES_AUTH_PROXY_LOGOUT_URL`    |                    | Where "Sign out" leads in proxy mode, e.g. `/oauth2/sign_out` |

Exactly one of the three password variables is required in password mode; without
authentication configured, the server does not start.

The port variable is deliberately not called `DENDRONOTES_PORT`: Kubernetes injects
`DENDRONOTES_PORT` and `DENDRONOTES_SERVICE_*` into every pod when a Service named
`dendronotes` exists. The manifests also set `enableServiceLinks: false`.

Operational endpoints: `/healthz` (liveness), `/readyz` (readiness; 503 while starting,
while shutting down, or when the soma is unreachable).

## Authentication

**Single user with a password** (default). Create a hash and pass it as a secret:

```bash
docker run --rm -it leafnotes:dev hash-password
```

Login attempts are rate-limited per client IP and globally; sessions are stored only as
SHA-256 hashes in `$DENDRONOTES_DATA_DIR/sessions.json` and sent as `HttpOnly`, `SameSite=Strict` cookies
(`__Host-` prefixed and `Secure` over HTTPS).

**Behind an authenticating proxy** (e.g. oauth2-proxy with Keycloak): set `DENDRONOTES_AUTH_MODE=proxy`,
`DENDRONOTES_TRUSTED_PROXIES` to the proxy's address range and `DENDRONOTES_AUTH_PROXY_HEADER` to the header it sets
(oauth2-proxy with `--set-xauthrequest`: `X-Auth-Request-User`). The header is ignored from any
other peer, so make sure only the proxy can reach leafnotes (Kubernetes: a NetworkPolicy).

## Security notes

- Every file access goes through Go's `os.Root`: paths with `..`, absolute paths and symlinks
  pointing out of the vault are refused by the kernel-level lookup. API paths are validated
  beforehand and can never address hidden entries (`.git`, `.trash`, …). Symlinks inside the
  vault are shown read-only.
- Saving requires the version the client started from (`If-Match`); changes made elsewhere in
  the meantime are never overwritten silently.
- State-changing requests from other origins are rejected (`Sec-Fetch-Site`/`Origin`); the API
  only accepts JSON bodies.
- Strict Content-Security-Policy with per-response nonce; no inline scripts, no `eval`, no
  external hosts.

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
| `POST /api/soma/move`                   | Rename or move: `{"from": …, "to": …}`               |
| `DELETE /api/soma/entries/{path}`       | Move a file or folder to `.trash/` in the soma       |

## Air gap and reproducible builds

```bash
make vendor          # vendor/ (Go) + .offline/npm-cache (npm, Linux binaries for amd64/arm64)
make image-offline   # rebuild without cache and without network in the build steps
```

- Base images are pinned by digest in the `Dockerfile`. In an air gap, mirror them into a local
  registry and pass `--build-arg NODE_IMAGE=… GO_IMAGE=… RUNTIME_IMAGE=…`.
- `make image` also writes `bin/leafnotes-<version>-<platform>.oci.tar` for transfer.
- Same source ⇒ same image digest (`SOURCE_DATE_EPOCH`, `rewrite-timestamp`, no provenance
  attestation with timestamps).
- `npm run build` fails if the bundle contains unexpected external URLs.

## Layout

```
cmd/leafnotes/    entry point: serve, hash-password, healthcheck, version
internal/config/  read and validate environment variables
internal/vault/   vault access exclusively through os.Root (tree, notes, move, trash)
internal/auth/    password (argon2id), sessions, rate limit, proxy header
internal/clientip/ client address behind trusted proxies
internal/httpx/   JSON helpers
internal/server/  routes, middleware (CSP with nonce, logging, recovery), health, assets
web/              Svelte 5 frontend (Vite), embedded into the binary via go:embed
```
