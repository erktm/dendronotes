import type { Change } from '../api/soma'
import { somaBase } from '../api/base'

export type LiveState = 'connecting' | 'live' | 'offline'

export type CollabEventName = 'collab' | 'presence' | 'saved' | 'notice'
const COLLAB_EVENTS: CollabEventName[] = ['collab', 'presence', 'saved', 'notice']

export interface LiveHandlers {
  /** Stream (re)connected: reload what is shown, something may have been missed. */
  resync(): void
  /** Something changed in the soma. */
  change(c: Change): void
  /** Members, roles or the soma itself changed: reload the soma list. */
  access(): void
  /** Editing together: updates, cursors, saves and notices of open notes. */
  collab(name: CollabEventName, data: unknown): void
  /** The stream was refused; the session may have ended. */
  refused(): void
}

const RETRY_MS = 5000

/**
 * The live connection to the server (Server-Sent Events). The browser
 * reconnects on its own after network errors; when the server refuses the
 * stream (e.g. signed out), we retry later ourselves.
 */
class Live {
  state = $state<LiveState>('offline')
  /** Increases with every change in the soma; views refetch derived data. */
  version = $state(0)
  /** Increases when the trash changed (restored, deleted for good). */
  trashVersion = $state(0)
  /** The note whose comments changed last, and a counter. */
  commentsChanged = $state.raw<{ path: string; n: number }>({ path: '', n: 0 })
  /** Increases when this account got a notification. */
  notificationVersion = $state(0)
  /** The id the server gave this stream ("hello"); notes are joined with it. */
  stream = $state('')

  private waiters: ((stream: string) => void)[] = []

  private source: EventSource | null = null
  private handlers: LiveHandlers | null = null
  private retry: ReturnType<typeof setTimeout> | undefined

  start(handlers: LiveHandlers): void {
    this.stop()
    this.handlers = handlers
    this.connect()
  }

  stop(): void {
    clearTimeout(this.retry)
    this.source?.close()
    this.source = null
    this.handlers = null
    this.state = 'offline'
    this.stream = ''
  }

  /** The stream id, once connected (waits for the next "hello" otherwise). */
  waitStream(): Promise<string> {
    if (this.state === 'live' && this.stream) return Promise.resolve(this.stream)
    return new Promise((resolve) => this.waiters.push(resolve))
  }

  private connect(): void {
    if (!this.handlers || typeof EventSource === 'undefined') return
    this.state = 'connecting'
    const es = new EventSource(`${somaBase()}/events`)
    this.source = es
    es.addEventListener('hello', (e) => {
      let stream = ''
      try {
        const data = JSON.parse((e as MessageEvent<string>).data) as { stream?: unknown }
        if (typeof data.stream === 'string') stream = data.stream
      } catch {
        // an older server: no editing together
      }
      this.stream = stream
      this.state = 'live'
      this.version++
      const waiting = this.waiters
      this.waiters = []
      for (const w of waiting) w(stream)
      this.handlers?.resync()
    })
    for (const name of COLLAB_EVENTS) {
      es.addEventListener(name, (e) => {
        let data: unknown
        try {
          data = JSON.parse((e as MessageEvent<string>).data)
        } catch {
          return
        }
        this.handlers?.collab(name, data)
      })
    }
    es.addEventListener('change', (e) => {
      let c: Change
      try {
        c = JSON.parse((e as MessageEvent<string>).data) as Change
      } catch {
        return
      }
      this.version++
      this.handlers?.change(c)
    })
    es.addEventListener('access', () => this.handlers?.access())
    es.addEventListener('trash', () => this.trashVersion++)
    es.addEventListener('comments', (e) => {
      try {
        const data = JSON.parse((e as MessageEvent<string>).data) as { path?: unknown }
        if (typeof data.path === 'string') this.commentsChanged = { path: data.path, n: this.commentsChanged.n + 1 }
      } catch {
        // ignore
      }
    })
    es.addEventListener('notification', () => this.notificationVersion++)
    es.onerror = () => {
      if (this.source !== es) return
      if (es.readyState === EventSource.CLOSED) {
        // Refused (401, 503 …): the browser gives up; try again later.
        es.close()
        this.source = null
        this.state = 'offline'
        this.handlers?.refused()
        clearTimeout(this.retry)
        this.retry = setTimeout(() => this.connect(), RETRY_MS)
      } else {
        this.state = 'connecting' // the browser reconnects by itself
      }
    }
  }
}

export const live = new Live()
