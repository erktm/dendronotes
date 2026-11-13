import { request } from './client'
import type { ShareRole, SomaRole } from '../roles'
import { encodePath } from '../paths'
import { somaBase } from './base'
import type { FolderOrder, Orders } from '../tree'

export interface Entry {
  path: string
  type: 'file' | 'dir'
  size?: number
  mtime: number
  /** When it was created, where the server knows it (tree only). */
  ctime?: number
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
  source: 'local' | 'oidc' | 'proxy' | 'ldap'
  disabled: boolean
  createdAt: string
  lastLoginAt: string
  avatar: Avatar
  /** Unread mentions and replies also by e-mail. */
  mailComments: boolean
  /** An e-mail when a soma is shared with the account. */
  mailShares: boolean
}

export type AvatarPattern = 'plain' | 'dendrite' | 'rings' | 'vesicles'

/** The designed part of an avatar. */
export interface AvatarStyle {
  /** Index into the avatar colors (0 … 7). */
  color: number
  pattern: AvatarPattern
  /** Up to two letters or digits; empty: taken from the name. */
  initials: string
}

/** How an account appears: its design and, if uploaded, a photo shown instead. */
export interface Avatar extends AvatarStyle {
  /** Version of the photo (GET /api/users/{id}/avatar?v=…), 0 when there is none. */
  photo: number
}

export interface Me {
  user: User
  mode: 'local' | 'proxy'
  logoutUrl?: string
  /** E-mail is configured on the server. */
  mail?: boolean
}

export type { ShareRole, SomaRole } from '../roles'

export interface Person {
  id: number
  username: string
  displayName: string
  /** Set in member lists. */
  avatar?: Avatar
}

/** A soma as the signed-in account sees it. */
export interface Membership {
  id: string
  name: string
  folder: string
  /** Effective: the highest of the personal share and the groups'. */
  role: SomaRole
  /** Shared personally (only then can it be left). */
  direct: boolean
  owner: Person
}

/** A set of accounts somas can be shared with; Everyone is every account. */
export interface Group {
  id: number
  name: string
  everyone: boolean
  /** ldap: mirrors an Active Directory group (name and members come from there). */
  source: 'local' | 'ldap'
  members: number
  createdAt: string
}

const file = (p: string) => `${somaBase()}/notes/${encodePath(p)}`

export const getTree = () =>
  request<{ entries: Entry[]; orders?: Orders }>('GET', `${somaBase()}/tree`).then((r) => ({ entries: r.entries, orders: r.orders ?? {} }))

/** Sorts a folder for everyone (editor). */
export const setOrder = (folder: string, order: FolderOrder) =>
  request<void>('PUT', `${somaBase()}/order`, { body: { folder, mode: order.mode, names: order.names ?? [] } })

export const readNote = (path: string) => request<Note>('GET', file(path))

export const saveNote = (path: string, content: string, etag: string) =>
  request<NoteMeta>('PUT', file(path), { body: { content }, headers: { 'If-Match': `"${etag}"` } })

export const createNote = (path: string, content = '') => request<NoteMeta>('POST', file(path), { body: { content } })

export const createFolder = (path: string) => request<Entry>('POST', `${somaBase()}/folders/${encodePath(path)}`, { body: {} })

export interface MoveResult {
  path: string
  updatedNotes?: number
  updatedSynapses?: number
  updatedEmbeds?: number
  failed?: string[]
}

/** Renames or moves; with updateSynapses, synapses, embeds and links to moved files are rewritten. */
export const move = (from: string, to: string, updateSynapses = false) =>
  request<MoveResult>('POST', `${somaBase()}/move`, { body: { from, to, updateSynapses } })

/** Uploads a file; with unique, a taken name gets a " (n)" suffix. Answers the stored entry. */
export const uploadFile = (path: string, file: Blob, unique = true) =>
  request<Entry>('POST', `${somaBase()}/files/${encodePath(path)}${unique ? '?unique=1' : ''}`, { raw: file })

export const trash = (path: string) =>
  request<{ trashedTo: string }>('DELETE', `${somaBase()}/entries/${encodePath(path)}`)

/** A deleted file or folder waiting in the soma's trash. */
export interface TrashItem {
  /** Relative to the trash. */
  path: string
  /** Where it was deleted from (restored there). */
  original: string
  type: 'file' | 'dir'
  size: number
  /** Folders: the files inside. */
  files?: number
  deletedAt: string
  /** Unknown for entries put into the trash elsewhere (Obsidian). */
  deletedBy: Person | null
}

const trashURL = (p: string) => `${somaBase()}/trash/${encodePath(p)}`

/** The trash, and after how many days entries are deleted for good (0: never). */
export const trashItems = () => request<{ items: TrashItem[]; keepDays: number }>('GET', `${somaBase()}/trash`)
export const readTrashNote = (path: string) => request<Note>('GET', trashURL(path))
export const restoreTrash = (path: string) => request<{ path: string }>('POST', trashURL(path))
export const purgeTrash = (path: string) => request<{ path: string }>('DELETE', trashURL(path))
export const emptyTrash = () => request<{ ok: boolean }>('DELETE', `${somaBase()}/trash`)

/** Why a version of a note was recorded. */
export type VersionReason = 'edit' | 'create' | 'outside' | 'relink' | 'restore'

export interface Version {
  id: number
  path: string
  at: string
  /** Accounts that changed the note in this version (see people). */
  authors: number[]
  reason: VersionReason
  size: number
  restoredFrom?: number
}

export const history = (path: string) =>
  request<{ versions: Version[]; people: Record<string, Person> }>('GET', `${somaBase()}/history/${encodePath(path)}`)
export const version = (id: number) => request<{ version: Version; content: string }>('GET', `${somaBase()}/versions/${id}`)
export const restoreVersion = (id: number) => request<NoteMeta>('POST', `${somaBase()}/versions/${id}/restore`)

/** Where the soma downloads as a ZIP. */
export const exportURL = (somaId: string) => `/api/somas/${encodeURIComponent(somaId)}/export`

export const me = () => request<Me>('GET', '/api/auth/me', { expect401: true })

export interface AuthMethods {
  mode: 'local' | 'proxy'
  /** The password form is offered to everyone (else only via ?local=1, for the admin). */
  local: boolean
  sso: { label: string } | null
  /** "Forgot password" is offered (e-mail is configured). */
  passwordReset?: boolean
  /** People may create accounts themselves (confirmed by e-mail). */
  signup?: boolean
}

export const authMethods = () => request<AuthMethods>('GET', '/api/auth/methods')

/** A banner as people see it (text is short Markdown, see lib/bannerText.ts). */
export interface ShownBanner {
  id: number
  message: string
  kind: 'info' | 'warning' | 'critical'
  dismissible: boolean
  /** Grows when the text changes; a closing counts for one revision. */
  revision: number
}

/** The banners of the sign-in page. */
export const loginBanners = () =>
  request<{ banners: ShownBanner[] }>('GET', '/api/auth/banners').then((r) => r.banners)
/** The banners the signed-in account sees now. */
export const myBanners = () => request<{ banners: ShownBanner[] }>('GET', '/api/me/banners').then((r) => r.banners)
/** Closes a banner for the account until its text changes. */
export const dismissBanner = (id: number) => request<{ ok: boolean }>('POST', `/api/me/banners/${id}/dismiss`)

/** Asks for a reset link; the answer is the same whether the account exists or not. */
export const forgotPassword = (login: string) => request<{ ok: boolean }>('POST', '/api/auth/password/forgot', { body: { login } })

/** Asks for an account: the address gets a link to confirm it. */
export const signup = (body: { username: string; displayName: string; email: string; password: string }) =>
  request<{ status: string }>('POST', '/api/auth/signup', { body })

/** Creates the account of a confirmed sign-up and signs it in. */
export const confirmSignup = (token: string) => request<Me>('POST', '/api/auth/signup/confirm', { body: { token } })

/** Sets a password with the token from a reset or welcome e-mail. */
export const resetPassword = (token: string, password: string) =>
  request<{ username: string }>('POST', '/api/auth/password/reset', { body: { token, password } })

export const login = (username: string, password: string) =>
  request<Me>('POST', '/api/auth/login', { body: { username, password }, expect401: true })

// --- Somas and profile -----------------------------------------------------------

export const listSomas = () => request<{ somas: Membership[] }>('GET', '/api/somas').then((r) => r.somas)
export const createSoma = (name: string) => request<Membership>('POST', '/api/somas', { body: { name } })
export const renameSoma = (id: string, name: string) =>
  request<Membership>('PATCH', `/api/somas/${encodeURIComponent(id)}`, { body: { name } })
/** Hands the soma to another member; the previous owner stays as editor. */
export const transferSoma = (id: string, ownerId: number) =>
  request<Membership>('PATCH', `/api/somas/${encodeURIComponent(id)}`, { body: { ownerId } })

// --- Sharing ---------------------------------------------------------------

export interface Member extends Person {
  role: SomaRole
}

export interface GroupGrant extends Group {
  role: ShareRole
}

export interface Members {
  members: Member[]
  groups: GroupGrant[]
  /** The caller's own (effective) role. */
  role: SomaRole
}

const membersURL = (id: string) => `/api/somas/${encodeURIComponent(id)}/members`
export const members = (id: string) => request<Members>('GET', membersURL(id))
export const setMember = (id: string, userId: number, role: ShareRole) =>
  request<Members>('PUT', `${membersURL(id)}/${userId}`, { body: { role } })
const groupsURL = (id: string) => `/api/somas/${encodeURIComponent(id)}/groups`
export const setSomaGroup = (id: string, groupId: number, role: ShareRole) =>
  request<Members>('PUT', `${groupsURL(id)}/${groupId}`, { body: { role } })
export const removeSomaGroup = (id: string, groupId: number) => request<Members>('DELETE', `${groupsURL(id)}/${groupId}`)
export const removeMember = (id: string, userId: number) => request<Members>('DELETE', `${membersURL(id)}/${userId}`)
export const leaveSoma = (id: string) => request<{ id: string }>('DELETE', `/api/somas/${encodeURIComponent(id)}/membership`)
/** People and groups to share with: at least two characters of a name. */
export const searchPeople = (q: string, signal?: AbortSignal) =>
  request<{ users: Person[]; groups: Group[] }>('GET', `/api/users/search?q=${encodeURIComponent(q)}`, { signal })
export const deleteSoma = (id: string) => request<{ id: string }>('DELETE', `/api/somas/${encodeURIComponent(id)}`)

export const updateProfile = (body: { displayName?: string; email?: string; mailComments?: boolean; mailShares?: boolean }) =>
  request<User>('PATCH', '/api/me', { body })
export const setAvatar = (style: AvatarStyle) => request<User>('PUT', '/api/me/avatar', { body: style })
/** A square PNG (64 … 512 pixels) the browser has cropped. */
export const uploadAvatarPhoto = (png: Blob) => request<User>('PUT', '/api/me/avatar/photo', { raw: png })
export const removeAvatarPhoto = () => request<User>('DELETE', '/api/me/avatar/photo')
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
  /** "drawing" for a drawing (*.drawing.svg). */
  kind?: 'drawing'
}

export interface GraphData {
  nodes: GraphNode[]
  /** Synapses. */
  edges: { source: string; target: string }[]
  /** Notes embedding or linking a drawing (not synapses). */
  embeds?: { source: string; target: string }[]
}

export const graph = (signal?: AbortSignal) => request<GraphData>('GET', `${somaBase()}/graph`, { signal })
