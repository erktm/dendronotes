import { request } from './client'
import { encodePath } from '../paths'
import { somaBase } from './base'

export interface Entry {
  path: string
  type: 'file' | 'dir'
  size?: number
  mtime: number
  readOnly?: boolean
}

export interface Note {
  path: string
  content: string
  etag: string
  size: number
  mtime: number
  readOnly?: boolean
}

export interface NoteMeta {
  path: string
  etag: string
  size: number
  mtime: number
}

export interface User {
  id: number
  username: string
  displayName: string
  email: string
  role: 'admin' | 'user'
  source: 'local' | 'oidc' | 'proxy'
  disabled: boolean
  createdAt: string
  lastLoginAt: string
}

export interface Me {
  user: User
  mode: 'local' | 'proxy'
  logoutUrl?: string
}

export type SomaRole = 'owner' | 'editor' | 'viewer'

export interface Person {
  id: number
  username: string
  displayName: string
}

/** A soma as the signed-in account sees it. */
export interface Membership {
  id: string
  name: string
  folder: string
  role: SomaRole
  owner: Person
}

const file = (p: string) => `${somaBase()}/notes/${encodePath(p)}`

export const getTree = () => request<{ entries: Entry[] }>('GET', `${somaBase()}/tree`).then((r) => r.entries)

export const readNote = (path: string) => request<Note>('GET', file(path))

export const saveNote = (path: string, content: string, etag: string) =>
  request<NoteMeta>('PUT', file(path), { body: { content }, headers: { 'If-Match': `"${etag}"` } })

export const createNote = (path: string, content = '') => request<NoteMeta>('POST', file(path), { body: { content } })

export const createFolder = (path: string) => request<Entry>('POST', `${somaBase()}/folders/${encodePath(path)}`, { body: {} })

export interface MoveResult {
  path: string
  updatedNotes?: number
  updatedSynapses?: number
  failed?: string[]
}

/** Renames or moves; with updateSynapses, synapses to moved notes are rewritten. */
export const move = (from: string, to: string, updateSynapses = false) =>
  request<MoveResult>('POST', `${somaBase()}/move`, { body: { from, to, updateSynapses } })

/** Uploads a file; with unique, a taken name gets a " (n)" suffix. Answers the stored entry. */
export const uploadFile = (path: string, file: Blob, unique = true) =>
  request<Entry>('POST', `${somaBase()}/files/${encodePath(path)}${unique ? '?unique=1' : ''}`, { raw: file })

export const trash = (path: string) =>
  request<{ trashedTo: string }>('DELETE', `${somaBase()}/entries/${encodePath(path)}`)

export const me = () => request<Me>('GET', '/api/auth/me', { expect401: true })

export const login = (username: string, password: string) =>
  request<Me>('POST', '/api/auth/login', { body: { username, password }, expect401: true })

// --- Somas and profile -----------------------------------------------------------

export const listSomas = () => request<{ somas: Membership[] }>('GET', '/api/somas').then((r) => r.somas)
export const createSoma = (name: string) => request<Membership>('POST', '/api/somas', { body: { name } })
export const renameSoma = (id: string, name: string) =>
  request<{ id: string; name: string }>('PATCH', `/api/somas/${encodeURIComponent(id)}`, { body: { name } })
export const deleteSoma = (id: string) => request<{ id: string }>('DELETE', `/api/somas/${encodeURIComponent(id)}`)

export const updateProfile = (body: { displayName?: string; email?: string }) => request<User>('PATCH', '/api/me', { body })
export const changePassword = (current: string, next: string) =>
  request<{ ok: boolean }>('POST', '/api/me/password', { body: { current, new: next } })

export const logout = () => request<{ logoutUrl?: string }>('POST', '/api/auth/logout', { expect401: true })

// --- Index -----------------------------------------------------------------

/** Encloses matched words in search snippets (never HTML). */
export const MARK_START = '\u0002'
export const MARK_END = '\u0003'

export interface Hit {
  path: string
  /** Text around the match, matches enclosed in MARK_START … MARK_END. */
  snippet: string
  /** First line with a match (0-based), -1 if only the name matched. */
  line: number
  tags?: string[]
}

export const search = (q: string, signal?: AbortSignal) =>
  request<{ hits: Hit[]; complete: boolean }>('GET', `${somaBase()}/search?q=${encodeURIComponent(q)}&limit=50`, { signal })

export interface Backlink {
  path: string
  synapses: { line: number; context: string; embed?: boolean }[]
}

export const backlinks = (path: string, signal?: AbortSignal) =>
  request<{ backlinks: Backlink[] }>('GET', `${somaBase()}/backlinks/${encodePath(path)}`, { signal }).then((r) => r.backlinks)

export interface TagCount {
  tag: string
  count: number
}

export const tags = (signal?: AbortSignal) => request<{ tags: TagCount[] }>('GET', `${somaBase()}/tags`, { signal }).then((r) => r.tags)

/** What changed in the soma, as sent by the event stream. */
export interface Change {
  tree: boolean
  notes: { path: string; etag: string }[]
  removed: string[]
}

export interface GraphNode {
  /** Soma path, or "?" + name for a note that synapses point to but that does not exist. */
  id: string
  label?: string
  missing?: boolean
  tags?: string[]
}

export interface GraphData {
  nodes: GraphNode[]
  edges: { source: string; target: string }[]
}

export const graph = (signal?: AbortSignal) => request<GraphData>('GET', `${somaBase()}/graph`, { signal })
