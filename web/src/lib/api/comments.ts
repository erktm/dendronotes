import { request } from './client'
import { somaBase } from './base'
import type { Person } from './soma'
import type { Anchor } from '../anchors'

export interface Comment extends Anchor {
  id: number
  path: string
  /** 0 for the first comment of a thread, else that comment's id. */
  threadId: number
  /** 0 when the account is gone. */
  authorId: number
  body: string
  createdAt: string
  editedAt?: string
  resolvedAt?: string
  resolvedBy?: number
}

export interface Notification {
  id: number
  kind: 'mention' | 'reply'
  somaId: string
  somaName: string
  path: string
  commentId: number
  threadId: number
  excerpt: string
  actor: Person | null
  createdAt: string
  read: boolean
}

const url = (id?: number) => `${somaBase()}/comments${id ? `/${id}` : ''}`

export const comments = (path: string) =>
  request<{ comments: Comment[]; people: Record<string, Person> }>('GET', `${url()}?path=${encodeURIComponent(path)}`)

/** A new thread on a note (with the text it is about), or a reply (threadId). */
export const addComment = (c: { path?: string; body: string; threadId?: number } & Partial<Anchor>) =>
  request<Comment>('POST', url(), { body: c })
export const editComment = (id: number, body: string) => request<Comment>('PATCH', url(id), { body: { body } })
export const deleteComment = (id: number) => request<{ id: number }>('DELETE', url(id))
export const resolveThread = (id: number, resolved: boolean) => request<Comment>('POST', `${url(id)}/resolve`, { body: { resolved } })

/** People with access to the soma, for @mentions. */
export const somaPeople = (q: string) =>
  request<{ people: Person[] }>('GET', `${somaBase()}/people?q=${encodeURIComponent(q)}`).then((r) => r.people)

export const notifications = () => request<{ notifications: Notification[]; unread: number }>('GET', '/api/me/notifications')
export const readNotifications = (ids?: number[]) =>
  request<{ unread: number }>('POST', '/api/me/notifications/read', { body: ids ? { ids } : {} })
