import { request } from './client'
import { encodePath } from '../paths'

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

export interface Me {
  user: string
  mode: 'password' | 'proxy'
  logoutUrl?: string
}

const file = (p: string) => `/api/soma/notes/${encodePath(p)}`

export const getTree = () => request<{ entries: Entry[] }>('GET', '/api/soma/tree').then((r) => r.entries)

export const readNote = (path: string) => request<Note>('GET', file(path))

export const saveNote = (path: string, content: string, etag: string) =>
  request<NoteMeta>('PUT', file(path), { body: { content }, headers: { 'If-Match': `"${etag}"` } })

export const createNote = (path: string, content = '') => request<NoteMeta>('POST', file(path), { body: { content } })

export const createFolder = (path: string) => request<Entry>('POST', `/api/soma/folders/${encodePath(path)}`, { body: {} })

export const move = (from: string, to: string) =>
  request<{ path: string }>('POST', '/api/soma/move', { body: { from, to } })

export const trash = (path: string) =>
  request<{ trashedTo: string }>('DELETE', `/api/soma/entries/${encodePath(path)}`)

export const me = () => request<Me>('GET', '/api/auth/me', { expect401: true })

export const login = (password: string) =>
  request<Me>('POST', '/api/auth/login', { body: { password }, expect401: true })

export const logout = () => request<{ logoutUrl?: string }>('POST', '/api/auth/logout', { expect401: true })
