// The soma the API calls below /api/somas/{soma}/ refer to.

let current = ''
// On the page of a public link, file URLs go through the link instead.
let publicToken = ''

/** Switches the soma that soma-scoped API calls and file URLs refer to. */
export function setSoma(id: string): void {
  current = id
}

/** The current soma's id ('' before one is chosen). */
export function currentSoma(): string {
  return current
}

/** Lets file URLs go through a public link (the page of /p/<token>). */
export function setPublicToken(token: string): void {
  publicToken = token
}

/** Base path of the current soma's API, e.g. /api/somas/k3xq7m2a9b, or of a public link. */
export function somaBase(): string {
  if (publicToken) return `/api/public/${encodeURIComponent(publicToken)}`
  return `/api/somas/${encodeURIComponent(current)}`
}
