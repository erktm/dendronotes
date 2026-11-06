// The soma the API calls below /api/somas/{soma}/ refer to.

let current = ''

/** Switches the soma that soma-scoped API calls and file URLs refer to. */
export function setSoma(id: string): void {
  current = id
}

/** The current soma's id ('' before one is chosen). */
export function currentSoma(): string {
  return current
}

/** Base path of the current soma's API, e.g. /api/somas/k3xq7m2a9b. */
export function somaBase(): string {
  return `/api/somas/${encodeURIComponent(current)}`
}
