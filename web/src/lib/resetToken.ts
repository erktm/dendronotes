// The token of a reset or welcome link travels in the URL fragment
// (/reset-password#<token>): it never reaches a server log or a Referer
// header. The page takes it once and removes it from the address bar and
// the history.

export const RESET_PATH = '/reset-password'

/** The token in a fragment like "#abc_-1", or "" if there is none. */
export function tokenFromHash(hash: string): string {
  const t = hash.startsWith('#') ? hash.slice(1) : hash
  return /^[A-Za-z0-9_-]{16,128}$/.test(t) ? t : ''
}

/** Reads the token of the current page and clears it from the URL. */
export function takeResetToken(): string {
  const token = tokenFromHash(location.hash)
  if (location.hash) history.replaceState(null, '', location.pathname)
  return token
}
