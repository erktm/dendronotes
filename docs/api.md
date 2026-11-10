# API

All endpoints return JSON. Everything that works on a soma lives below
`/api/somas/{soma}/` (`{soma}` is the soma's id); paths inside the soma are soma-relative and
URL-encoded per segment. The full description is in [`openapi.yaml`](openapi.yaml),
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
| `GET /api/somas/{soma}/drawings/{path}` | Read a drawing (`*.drawing.svg`, with `ETag`); `POST` creates an empty one, `PUT` saves (`If-Match`, `412` on conflict, checked SVG) |
| `GET /api/somas/{soma}/icons`           | The icon library; `POST` adds a picture (raw body, `?name=`), `GET`/`DELETE …/icons/{id}` |
| `GET /api/somas/{soma}/files/{path}`    | Download a file (attachments, images, media)         |
| `POST /api/somas/{soma}/files/{path}`   | Upload a file (raw body; `?unique=1` avoids name clashes) |
| `GET /api/somas/{soma}/search?q=`       | Full-text search (words, "phrases", #tags)           |
| `GET /api/somas/{soma}/backlinks/{path}` | Notes with synapses to a note                       |
| `GET /api/somas/{soma}/tags`            | All tags with their number of notes                  |
| `GET /api/somas/{soma}/graph`           | All notes and the synapses between them              |
| `GET /api/somas/{soma}/events`          | Live updates (Server-Sent Events)                    |
| `POST /api/somas/{soma}/collab/join`, `/leave`, `/push`, `/presence`; `GET …/collab/pull` | Editing together: open a note with the stream's id, send changes (CodeMirror change sets), cursors, catch up |
| `GET /api/somas/{soma}/members`         | Who has access (people and groups), with roles     |
| `PUT`, `DELETE /api/somas/{soma}/members/{id}` | Share with an account or change its role; take access away (owner) |
| `PUT`, `DELETE /api/somas/{soma}/groups/{id}` | Share with a group or change its role; take access away (owner) |
| `DELETE /api/somas/{soma}/membership`   | Leave a soma shared with you personally              |
| `GET /api/users/search?q=`              | Find people to share with (two characters or more)   |
| `GET /api/auth/methods`, `/api/auth/oidc/login`, `/api/auth/oidc/callback` | Sign-in options and the OpenID Connect flow |
| `/api/admin/users`, `/groups`, `/somas`, `/sessions`, `/server`, `/sso/check`, `/audit` | Administration (admins only) |

**Personal API tokens** (profile → *API tokens*) let scripts use the API as their account:
"Read" tokens only read, "Read and write" tokens also change notes and files. No token manages
anything: administration, sharing, renaming or deleting somas, the account and its tokens
answer `403 token_not_allowed`. A token is shown once; only its hash is stored.

```bash
curl -H "Authorization: Bearer dn_…" https://notes.example.org/api/somas
```
