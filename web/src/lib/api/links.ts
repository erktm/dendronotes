// Links to notes and drawings that follow them when they move: the internal
// link of an entry (everyone with access) and public read-only links.
import { request } from './client'
import { somaBase } from './base'

export interface ShareLink {
  id: string
  path: string
  kind: 'internal' | 'public'
  createdBy?: { id: number; username: string; displayName: string }
  createdAt: string
  expiresAt?: string
}

/** The internal link of a file (made on first use). */
export const internalLink = (path: string) => request<ShareLink>('POST', `${somaBase()}/links`, { body: { path } })

/** Where an internal link points now. */
export const openLink = (id: string) => request<ShareLink>('GET', `${somaBase()}/links/${encodeURIComponent(id)}`)

/** The soma's public links (editors). */
export const publicLinks = () => request<{ links: ShareLink[] }>('GET', `${somaBase()}/shares`).then((r) => r.links)

/** Makes a public link; its token is in this answer only. days 0: it does not expire. */
export const createPublicLink = (path: string, days: number) =>
  request<{ token: string; link: ShareLink }>('POST', `${somaBase()}/shares`, { body: { path, days } })

export const revokePublicLink = (id: string) => request<void>('DELETE', `${somaBase()}/shares/${encodeURIComponent(id)}`)

/** What a public link shows (no sign-in). */
export interface Shared {
  path: string
  kind: 'note' | 'drawing'
  content?: string
  files?: string[]
  svg?: string
}

export const readShared = (token: string) => request<Shared>('GET', `/api/public/${encodeURIComponent(token)}`, { expect401: true })

/** The address of an internal link and of a public one. */
export const internalURL = (soma: string, id: string) => `${location.origin}/s/${soma}/l/${id}`
export const publicURL = (token: string) => `${location.origin}/p/${token}`
