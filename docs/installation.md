# Installation and operation

dendronotes is one program: a Go binary with the web interface built in. It keeps the notes as
plain files in a folder (the *somas*, one subfolder per soma) and its own state (accounts,
shares, histories, search indexes) in a data folder. There are three ways to run it:

| Way | For | Artifact |
|-----|-----|----------|
| [Binary](#binary) | a single server or your own computer | `dendronotes_<version>_<os>_<arch>.tar.gz` from the [releases](https://github.com/erktm/dendronotes/releases) |
| [Container](#container) | Docker, Podman, Docker Compose | `ghcr.io/erktm/dendronotes:<version>` (linux/amd64, linux/arm64) |
| [Helm chart](#kubernetes-with-helm) | Kubernetes | `oci://ghcr.io/erktm/charts/dendronotes` |

Whatever the way, dendronotes needs:

- a **somas folder** (`DENDRONOTES_SOMAS_DIR`) and a **data folder** (`DENDRONOTES_DATA_DIR`),
  both writable, the data folder outside the somas;
- on the first start an **admin password**, given by one of `DENDRONOTES_ADMIN_PASSWORD_FILE` (a file
  with the password, at least 10 characters), `DENDRONOTES_ADMIN_PASSWORD_HASH` or
  `DENDRONOTES_ADMIN_PASSWORD_HASH_FILE` (an argon2id hash from `dendronotes hash-password`).
  Once the admin account exists, they are ignored;
- for anything beyond your own computer: **HTTPS** in front (a reverse proxy, ingress or
  gateway) and `DENDRONOTES_PUBLIC_URL` set to the address people use.

All settings are `DENDRONOTES_*` environment variables, listed in the
[README](../README.md#configuration). Signing in through Keycloak or Active Directory is
described in [authentication](authentication.md).

## Binary

Releases provide binaries for Linux and macOS on amd64 and arm64, with a `checksums.txt` and a
software bill of materials. They need nothing else: no runtime, no database server.

```bash
VERSION=0.1.0
curl -LO https://github.com/erktm/dendronotes/releases/download/v$VERSION/dendronotes_${VERSION}_linux_amd64.tar.gz
curl -LO https://github.com/erktm/dendronotes/releases/download/v$VERSION/checksums.txt
sha256sum --check --ignore-missing checksums.txt
tar -xzf dendronotes_${VERSION}_linux_amd64.tar.gz
sudo install dendronotes /usr/local/bin/
```

Try it in a folder of your own:

```bash
mkdir -p ~/dendronotes/somas ~/dendronotes/data
head -c 18 /dev/urandom | base64 > ~/dendronotes/admin-password   # or write your own
chmod 600 ~/dendronotes/admin-password

DENDRONOTES_SOMAS_DIR=~/dendronotes/somas \
DENDRONOTES_DATA_DIR=~/dendronotes/data \
DENDRONOTES_ADMIN_PASSWORD_FILE=~/dendronotes/admin-password \
DENDRONOTES_LOG_FORMAT=text \
dendronotes serve
```

Then open <http://localhost:8080> and sign in as `admin` with that password. On macOS a
downloaded binary is quarantined; allow it once with `xattr -d com.apple.quarantine
dendronotes`.

An existing folder of Markdown files, for example one you use with Obsidian, becomes the admin's first soma
with `DENDRONOTES_SOMA_DIR=/path/to/notes`; the files stay where they are.

### As a service (systemd)

```ini
# /etc/systemd/system/dendronotes.service
[Unit]
Description=dendronotes
After=network-online.target
Wants=network-online.target

[Service]
User=dendronotes
Group=dendronotes
Environment=DENDRONOTES_SOMAS_DIR=/var/lib/dendronotes/somas
Environment=DENDRONOTES_DATA_DIR=/var/lib/dendronotes/data
Environment=DENDRONOTES_ADMIN_PASSWORD_FILE=/etc/dendronotes/admin-password
Environment=DENDRONOTES_PUBLIC_URL=https://notes.example.org
Environment=DENDRONOTES_TRUSTED_PROXIES=127.0.0.1/32
ExecStart=/usr/local/bin/dendronotes serve
Restart=on-failure
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/dendronotes

[Install]
WantedBy=multi-user.target
```

```bash
sudo useradd --system --home /var/lib/dendronotes dendronotes
sudo install -d -o dendronotes -g dendronotes /var/lib/dendronotes/somas /var/lib/dendronotes/data
sudo install -d /etc/dendronotes
head -c 18 /dev/urandom | base64 | sudo install -m 600 -o dendronotes /dev/stdin /etc/dendronotes/admin-password
sudo systemctl daemon-reload && sudo systemctl enable --now dendronotes
```

Put a reverse proxy with TLS in front (see [reverse proxy](#reverse-proxy)).

## Container

The image has a distroless base (no shell, no package manager) and runs as UID 65532 with a
read-only root file system and without capabilities.

```bash
mkdir -p somas && sudo chown 65532:65532 somas    # the container's user must be able to write
head -c 18 /dev/urandom | base64 > admin-password && chmod 644 admin-password

docker run -d --name dendronotes -p 8080:8080 \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  -v "$PWD/somas:/somas" -v dendronotes-data:/data \
  -v "$PWD/admin-password:/run/secrets/admin-password:ro" \
  -e DENDRONOTES_ADMIN_PASSWORD_FILE=/run/secrets/admin-password \
  ghcr.io/erktm/dendronotes:0.1.0
```

`/somas` holds the notes (one folder per soma), `/data` the account database `state.db`, the
histories and the search indexes. Instead of a password file you can pass a hash:
`docker run --rm -it ghcr.io/erktm/dendronotes:0.1.0 hash-password` prints one for
`DENDRONOTES_ADMIN_PASSWORD_HASH`.

### Docker Compose

```yaml
services:
  dendronotes:
    image: ghcr.io/erktm/dendronotes:0.1.0
    restart: unless-stopped
    ports:
      - "8080:8080"
    read_only: true
    cap_drop: [ALL]
    security_opt: ["no-new-privileges:true"]
    environment:
      DENDRONOTES_ADMIN_PASSWORD_FILE: /run/secrets/admin_password
      DENDRONOTES_PUBLIC_URL: https://notes.example.org
      DENDRONOTES_TRUSTED_PROXIES: 172.16.0.0/12
    volumes:
      - ./somas:/somas
      - data:/data
    secrets:
      - admin_password

secrets:
  admin_password:
    file: ./admin-password

volumes:
  data:
```

## Kubernetes with Helm

The chart lives in [`deploy/helm/dendronotes`](../deploy/helm/dendronotes) and is published
with every release:

```bash
helm install dendronotes oci://ghcr.io/erktm/charts/dendronotes --version 0.1.0 \
  --namespace dendronotes --create-namespace -f my-values.yaml
```

It creates a Deployment `dendronotes` (one pod, `Recreate`, because dendronotes is the only writer of
its files), a Service, the PersistentVolumeClaims `dendronotes-somas` and `dendronotes-data`
(kept on uninstall), the Secret `dendronotes-auth` and, if asked for, an Ingress or a Gateway
API HTTPRoute. The pod runs as UID 65532 with a read-only root file system, no capabilities
and no service links (Kubernetes would otherwise inject `DENDRONOTES_PORT`).

Without an admin password in the values the chart generates one and keeps it across
upgrades; the notes after `helm install` show how to read it:

```bash
kubectl -n dendronotes get secret dendronotes-auth -o jsonpath='{.data.admin-password}' | base64 -d; echo
```

An example with Keycloak, Longhorn and Envoy Gateway:

```yaml
# my-values.yaml
config:
  DENDRONOTES_PUBLIC_URL: https://notes.example.org
  DENDRONOTES_OIDC_ISSUER: https://sso.example.org/realms/main
  DENDRONOTES_OIDC_CLIENT_ID: dendronotes
  DENDRONOTES_OIDC_ADMIN_GROUPS: dendronotes-admins
  DENDRONOTES_OIDC_LOGIN_LABEL: Keycloak
  DENDRONOTES_TRUSTED_PROXIES: 10.0.0.0/8

auth:
  # A Secret you manage yourself (e.g. a Sealed Secret) with the keys
  # admin-password and oidc-client-secret.
  existingSecret: dendronotes-auth
  oidcClientSecretKey: oidc-client-secret

persistence:
  somas:
    storageClass: longhorn
    size: 20Gi
  data:
    storageClass: longhorn

httpRoute:
  enabled: true
  parentRefs:
    - name: eg
      namespace: envoy-gateway-system
  hostnames:
    - notes.example.org
```

With an Ingress instead, set `ingress.enabled: true` with your class and host; for
ingress-nginx turn buffering off and allow long reads for the live updates (the commented
annotations in [`values.yaml`](../deploy/helm/dendronotes/values.yaml)). `metrics.enabled: true`
adds the Prometheus port and a Service `dendronotes-metrics`. All values are documented in
`values.yaml`; `helm show values oci://ghcr.io/erktm/charts/dendronotes` prints them.

Behind a registry mirror or with an image of your own, set `image.repository` and, if it needs
credentials, `imagePullSecrets`.

## Reverse proxy

dendronotes speaks plain HTTP; TLS belongs to the proxy, ingress or gateway in front of it.

- Set `DENDRONOTES_PUBLIC_URL` to the external address and `DENDRONOTES_TRUSTED_PROXIES` to the
  proxy's address range, so client addresses and HTTPS are recognized (cookies are then sent
  with `Secure` and the `__Host-` prefix).
- The live updates are Server-Sent Events on `/api/somas/{soma}/events`: no buffering for that
  path and idle reads of at least 30 seconds (a heartbeat comes every 25 seconds).
  dendronotes sets `X-Accel-Buffering: no`, which nginx honors.
- Uploads are limited by `DENDRONOTES_MAX_UPLOAD_MB` (25 MB); allow at least that body size.

A minimal Caddy configuration does all of this: `notes.example.org { reverse_proxy
localhost:8080 }`.

## E-mail

With an SMTP relay, dendronotes sends e-mails: to people a soma was shared with, about
mentions and replies that are still unread after a while (bundled into one message), security
notices (password changed, new API token, e-mail address changed, sent to the old address),
welcome e-mails for accounts an admin creates, and the links of "forgot password". Without
`DENDRONOTES_SMTP_HOST` all of this stays off and invisible. `DENDRONOTES_PUBLIC_URL` is
required with it, for the links.

An example with Mailjet (port 587 with STARTTLS; the API key is the user name, the secret key
the password):

```bash
DENDRONOTES_PUBLIC_URL=https://notes.example.org
DENDRONOTES_SMTP_HOST=in-v3.mailjet.com
DENDRONOTES_SMTP_PORT=587
DENDRONOTES_SMTP_TLS=starttls
DENDRONOTES_SMTP_USERNAME=<API key>
DENDRONOTES_SMTP_PASSWORD_FILE=/run/secrets/smtp-password
DENDRONOTES_MAIL_FROM="dendronotes <no-reply@example.org>"
```

With Helm, the same goes into `mail` and the password into the Secret:

```yaml
mail:
  host: in-v3.mailjet.com
  username: <API key>
  from: "dendronotes <no-reply@example.org>"
auth:
  smtpPassword: <secret key>   # or existingSecret with smtpPasswordKey: smtp-password
```

The relay must accept the sender: confirm the address or the whole domain at your provider and
publish its SPF and DKIM records in DNS, otherwise the messages are refused or end up as spam.
Other relays work the same way; `DENDRONOTES_SMTP_TLS=tls` is for implicit TLS (port 465),
`none` only for a relay without a password in your own network.

Messages wait in an outbox in `state.db` until the relay took them; failed attempts are
repeated after 1, 5 and 30 minutes, then hourly, and given up after a day (or at once when the
relay refuses a recipient for good). Sent messages are deleted. *Admin → Server → E-mail* shows
the relay, the waiting messages and the last error, and sends a test e-mail to your own address.
Everyone chooses in the profile which notifications they also get by e-mail; security notices
always go out. `DENDRONOTES_MAIL_DIGEST_DELAY` (10 minutes) sets how long a mention may stay
unread before it is mailed.

## Operation

**Backups** need both: the somas directory and what in `/data` is not a cache, namely `state.db`
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
metrics on that port. It serves only `/metrics`, nothing else; keep the port internal (a separate
container port, not routed through the ingress): requests by route pattern, method and status
with durations, sign-ins by way and result, accounts, sessions, somas, open somas, live
connections, notes being edited, disk usage, whether the OpenID Connect provider and the
directory answer, the last directory sync, and Go runtime figures. Note paths never become
labels.

**Locked out?** `dendronotes reset-password [user]` sets a new password for a local account
(by default the admin account), enables it and ends its sessions, also while the server runs:

```bash
docker exec -it dendronotes dendronotes reset-password
```

**Health.** `/healthz` answers while the process runs (liveness), `/readyz` only when the
account database and the somas folder are reachable (readiness; 503 while starting or shutting
down). The container image uses `dendronotes healthcheck`.

**Upgrading.** Replace the binary or the image tag (`helm upgrade` with the new chart version)
and start again; there is one process, so expect a few seconds without service. Database
changes migrate on start after a backup copy (`state.db.v<n>.bak`); the search indexes rebuild
themselves when their format changes.

## Building from source

Requirements: Go (see `go.mod`), Node 26 with npm, Docker for the image.

```bash
make build        # bin/dendronotes with the embedded web interface
make image        # the container image for this machine, loaded into Docker
make test         # all tests and checks
helm package deploy/helm/dendronotes
```

### Air gap and reproducible builds

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
