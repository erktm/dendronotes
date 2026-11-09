import { request } from './client'
import type { Group, Person, ShareRole, User } from './soma'

export interface AdminSoma {
  id: string
  name: string
  folder: string
  ownerId: number
  createdAt: string
  owner: Person
  members: number
  /** Groups the soma is shared with. */
  groups: number
  path: string
  bytes: number
  files: number
}

export interface Session {
  id: number
  userId: number
  username: string
  createdAt: string
  expiresAt: string
  seenAt: string
  ip: string
  userAgent: string
}

export interface AuditEntry {
  id: number
  at: string
  actorId: number
  actor: string
  action: string
  target: string
  detail: string
  ip: string
}

export interface ServerInfo {
  version: string
  goVersion: string
  platform: string
  startedAt: string
  uptime: string
  authMode: string
  counts: { users: number; somas: number; sessions: number; openSomas: number }
  open: { id: string; name: string; notes: number; streams: number; editing: number; ready: boolean }[]
  storage: { somasDir: string; somasBytes: number; somasFiles: number; dataDir: string; dataBytes: number }
  limits: { maxNoteBytes: number; maxUploadBytes: number }
  settings: Record<string, string>
  localLogin: boolean
  sso: SSOStatus | null
  ldap: LDAPStatus | null
}

export interface SSOStatus {
  issuer: string
  clientId: string
  redirectUrl: string
  ready: boolean
  checkedAt: string
  error?: string
  logout: boolean
}

export interface LDAPStatus {
  url: string
  baseDN: string
  bindDN: string
  groupBaseDN: string
  ready: boolean
  /** Zero time until the first check has finished. */
  checkedAt: string
  error?: string
}

export interface NewUser {
  username: string
  displayName: string
  email: string
  password: string
  role: 'admin' | 'user'
}

export const users = () => request<{ users: User[] }>('GET', '/api/admin/users').then((r) => r.users)
export const createUser = (u: NewUser) => request<User>('POST', '/api/admin/users', { body: u })
export const updateUser = (id: number, body: Partial<Pick<User, 'displayName' | 'email' | 'role' | 'disabled'>>) =>
  request<User>('PATCH', `/api/admin/users/${id}`, { body })
export const setPassword = (id: number, password: string) =>
  request<{ ok: boolean }>('POST', `/api/admin/users/${id}/password`, { body: { password } })
export const deleteUser = (id: number, transferTo?: number) =>
  request<{ somas: number }>('DELETE', `/api/admin/users/${id}${transferTo ? `?transferTo=${transferTo}` : ''}`)
export const endUserSessions = (id: number) => request<{ ok: boolean }>('DELETE', `/api/admin/users/${id}/sessions`)

export const somas = () => request<{ somas: AdminSoma[] }>('GET', '/api/admin/somas').then((r) => r.somas)

export const groups = () => request<{ groups: Group[] }>('GET', '/api/admin/groups').then((r) => r.groups)
export const createGroup = (name: string) => request<Group>('POST', '/api/admin/groups', { body: { name } })
export const renameGroup = (id: number, name: string) => request<Group>('PATCH', `/api/admin/groups/${id}`, { body: { name } })
export const deleteGroup = (id: number) => request<{ id: number }>('DELETE', `/api/admin/groups/${id}`)
export interface GroupMembers {
  group: Group
  members: Person[]
}
export const groupMembers = (id: number) => request<GroupMembers>('GET', `/api/admin/groups/${id}/members`)
export const addGroupMember = (id: number, userId: number) =>
  request<GroupMembers>('PUT', `/api/admin/groups/${id}/members/${userId}`)
export const removeGroupMember = (id: number, userId: number) =>
  request<GroupMembers>('DELETE', `/api/admin/groups/${id}/members/${userId}`)
export interface NewSoma {
  name: string
  ownerId: number
  /** Shared with this group right away. */
  group?: { id: number; role: ShareRole }
}
export const createSoma = (s: NewSoma) => request<AdminSoma>('POST', '/api/admin/somas', { body: s })
export const transferSoma = (id: string, ownerId: number) =>
  request<{ id: string }>('PATCH', `/api/admin/somas/${encodeURIComponent(id)}`, { body: { ownerId } })
export const deleteSoma = (id: string) => request<{ id: string }>('DELETE', `/api/admin/somas/${encodeURIComponent(id)}`)

/** A deleted soma an admin can bring back. */
export interface DeletedSoma {
  id: string
  name: string
  folder: string
  originalFolder: string
  /** null when the owner's account is gone (choose another to restore). */
  owner: Person | null
  members: number
  groups: number
  createdAt: string
  deletedBy: Person | null
  deletedAt: string
}

export const deletedSomas = () => request<{ somas: DeletedSoma[] }>('GET', '/api/admin/somas/deleted').then((r) => r.somas)
export const restoreSoma = (id: string, ownerId?: number) =>
  request<{ id: string }>('POST', `/api/admin/somas/deleted/${encodeURIComponent(id)}/restore`, { body: ownerId ? { ownerId } : {} })
export const purgeSoma = (id: string) => request<{ id: string }>('DELETE', `/api/admin/somas/deleted/${encodeURIComponent(id)}`)

/** Where the backup of state.db and the histories downloads. */
export const backupURL = '/api/admin/backup'

export const sessions = () => request<{ sessions: Session[]; current: number }>('GET', '/api/admin/sessions')
export const endSession = (id: number) => request<{ ok: boolean }>('DELETE', `/api/admin/sessions/${id}`)

export const server = () => request<ServerInfo>('GET', '/api/admin/server')
export const checkSSO = () => request<SSOStatus>('POST', '/api/admin/sso/check')
export const checkLDAP = () => request<LDAPStatus>('POST', '/api/admin/ldap/check')

export const audit = (before = 0, limit = 100) =>
  request<{ entries: AuditEntry[] }>('GET', `/api/admin/audit?limit=${limit}${before ? `&before=${before}` : ''}`).then((r) => r.entries)

/** "Nov 6, 2026, 10:24 AM" – English like the rest of the UI. */
const dateTime = new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' })
export function formatDate(iso: string): string {
  const d = new Date(iso)
  return isNaN(d.getTime()) || d.getTime() <= 0 ? '' : dateTime.format(d)
}
