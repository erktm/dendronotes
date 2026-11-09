// The comments on the open note: threads, who wrote them, which one is in
// focus, and a draft for a new thread on selected text.

import * as api from '../api/comments'
import type { Comment } from '../api/comments'
import type { Person } from '../api/soma'
import type { Anchor } from '../anchors'

export interface Thread {
  root: Comment
  replies: Comment[]
}

class Comments {
  /** The note the comments belong to. */
  path = $state('')
  list = $state.raw<Comment[]>([])
  people = $state.raw<Record<string, Person>>({})
  loaded = $state(false)
  /** The thread in focus (clicked in the note or opened from a notification). */
  active = $state<number | null>(null)
  /** A new thread being written, on this text (null: about the whole note). */
  draft = $state.raw<Anchor | null>(null)
  /** Bumped to move the focus into the new-comment field. */
  draftFocus = $state(0)
  /** Bumped to scroll the active thread's passage into view in the note. */
  reveal = $state(0)

  threads = $derived.by(() => {
    const byRoot = new Map<number, Thread>()
    for (const c of this.list) if (c.threadId === 0) byRoot.set(c.id, { root: c, replies: [] })
    for (const c of this.list) if (c.threadId !== 0) byRoot.get(c.threadId)?.replies.push(c)
    return [...byRoot.values()]
  })

  private loading = 0

  /** Loads the comments of path (again). */
  async load(path: string): Promise<void> {
    if (path !== this.path) {
      this.path = path
      this.list = []
      this.loaded = false
      this.draft = null
    }
    const n = ++this.loading
    try {
      const r = await api.comments(path)
      if (n !== this.loading || path !== this.path) return
      this.list = r.comments
      this.people = r.people
      this.loaded = true
    } catch {
      if (n === this.loading) this.loaded = true
    }
  }

  clear(): void {
    this.path = ''
    this.list = []
    this.loaded = false
    this.draft = null
  }

  /** Puts a thread in focus and shows its passage in the note. */
  show(id: number): void {
    this.active = id
    this.reveal++
  }

  person(id: number | undefined): Person | null {
    return id ? (this.people[String(id)] ?? null) : null
  }
}

export const comments = new Comments()
