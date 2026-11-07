// Editing together: see internal/collab. A tab joins a note with the id of
// its event stream and then pushes CodeMirror change sets for the version it
// has seen; everyone's updates come back on the event stream.

import { request } from './client'
import { somaBase } from './base'

export interface WireUpdate {
  clientID: string
  /** ChangeSet.toJSON() */
  changes: unknown
}

export interface Peer {
  id: string
  name: string
  /** Index into the fixed cursor palette (peer-c0 … peer-c7). */
  color: number
  sel?: { anchor: number; head: number }
}

export interface Joined {
  doc: string
  version: number
  text: string
  readOnly: boolean
  /** Version written to the file. */
  saved: number
  peer: string
  peers: Peer[]
  conflict?: boolean
}

/** Local changes made against a document the server no longer has. */
export interface Recover {
  doc: string
  base: string
  mine: string
}

// Events on the stream (all carry the note's path and document id).
export interface CollabEvent {
  path: string
  doc: string
  from: number
  updates: WireUpdate[]
}
export interface PresenceEvent {
  path: string
  doc: string
  peers: Peer[]
}
export interface SavedEvent {
  path: string
  doc: string
  version: number
}
export interface NoticeEvent {
  path: string
  doc: string
  kind: 'renamed' | 'removed' | 'merged' | 'conflict' | 'save_failed' | 'read_only'
  from?: string
  to?: string
}

const url = (op: string) => `${somaBase()}/collab/${op}`

export const join = (path: string, stream: string, recover?: Recover) =>
  request<Joined>('POST', url('join'), { body: { path, stream, recover } })

export const leave = (path: string, stream: string) =>
  request<{ ok: boolean }>('POST', url('leave'), { body: { path, stream } })

export const push = (path: string, stream: string, doc: string, version: number, updates: WireUpdate[], save = false) =>
  request<{ version: number }>('POST', url('push'), { body: { path, stream, doc, version, updates, save } })

export const pull = (path: string, doc: string, version: number) =>
  request<{ updates: WireUpdate[]; version: number }>(
    'GET',
    `${url('pull')}?path=${encodeURIComponent(path)}&doc=${encodeURIComponent(doc)}&version=${version}`,
  )

export const presence = (path: string, stream: string, doc: string, sel: { anchor: number; head: number } | null) =>
  request<{ ok: boolean }>('POST', url('presence'), { body: { path, stream, doc, sel } })
