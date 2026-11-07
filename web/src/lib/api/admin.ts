import { request } from './client'
import type { Person, User } from './soma'

export interface AdminSoma {
  id: string
  name: string
  folder: string
  ownerId: number
  createdAt: string
  owner: Person
  members: number
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
  open: { id: string; name: string; notes: number; streams: number; ready: boolean }[]
  storage: { somasDir: string; somasBytes: number; somasFiles: number; dataDir: string; dataBytes: number }
  limits: { maxNoteBytes: number; maxUploadBytes: number }
  settings: Record<string, string>
  localLogin: boolean
  sso: SSOStatus | null
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
export const transferSoma = (id: string, ownerId: number) =>
  request<{ id: string }>('PATCH', `/api/admin/somas/${encodeURIComponent(id)}`, { body: { ownerId } })
export const deleteSoma = (id: string) => request<{ id: string }>('DELETE', `/api/admin/somas/${encodeURIComponent(id)}`)

export const sessions = () => request<{ sessions: Session[]; current: number }>('GET', '/api/admin/sessions')
export const endSession = (id: number) => request<{ ok: boolean }>('DELETE', `/api/admin/sessions/${id}`)

export const server = () => request<ServerInfo>('GET', '/api/admin/server')
export const checkSSO = () => request<SSOStatus>('POST', '/api/admin/sso/check')

export const audit = (before = 0, limit = 100) =>
  request<{ entries: AuditEntry[] }>('GET', `/api/admin/audit?limit=${limit}${before ? `&before=${before}` : ''}`).then((r) => r.entries)

/** "Nov 6, 2026, 10:24 AM" – English like the rest of the UI. */
const dateTime = new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' })
export function formatDate(iso: string): string {
  const d = new Date(iso)
  return isNaN(d.getTime()) || d.getTime() <= 0 ? '' : dateTime.format(d)
}
