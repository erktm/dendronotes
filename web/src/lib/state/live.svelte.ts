import type { Change } from '../api/soma'
import { somaBase } from '../api/base'

export type LiveState = 'connecting' | 'live' | 'offline'

export type CollabEventName = 'collab' | 'presence' | 'saved' | 'notice'
const COLLAB_EVENTS: CollabEventName[] = ['collab', 'presence', 'saved', 'notice']
const EVENT_NAMES = ['hello', ...COLLAB_EVENTS, 'change', 'access', 'trash', 'comments', 'notification', 'banners']

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
/** Without a "hello" this soon, the network holds the stream back: poll instead. */
export const HELLO_TIMEOUT_MS = 8000
/** How long this browser remembers that it has to poll. */
const POLL_MEMORY_MS = 7 * 24 * 60 * 60 * 1000
const POLL_KEY = 'dendronotes.livePolling'

interface Polled {
  stream: string
  events: { seq: number; name: string; data: unknown }[]
}

function pollingRemembered(): boolean {
  try {
    const since = Number(localStorage.getItem(POLL_KEY))
    return since > 0 && Date.now() - since < POLL_MEMORY_MS
  } catch {
    return false
  }
}

function rememberPolling(): void {
  try {
    localStorage.setItem(POLL_KEY, String(Date.now()))
  } catch {
    // storage disabled: poll again after the next timeout
  }
}

const sleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve) => {
    const t = setTimeout(resolve, ms)
    signal.addEventListener('abort', () => (clearTimeout(t), resolve()), { once: true })
  })

/**
 * The live connection to the server. It uses Server-Sent Events; the browser
 * reconnects on its own after network errors, and when the server refuses the
 * stream (e.g. signed out), we retry later ourselves. Some networks hold a
 * stream back until it ends (proxies or security software that pass on only
 * complete responses): when no "hello" arrives in time, the same events come
 * by long polling instead, and this browser remembers that for a while.
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
  /** Grows when the admins change banners. */
  bannersVersion = $state(0)
  /** The id the server gave this stream ("hello"); notes are joined with it. */
  stream = $state('')
  /** How the events arrive: a stream, or by long polling. */
  transport = $state<'stream' | 'poll'>('stream')

  private waiters: ((stream: string) => void)[] = []

  private source: EventSource | null = null
  private handlers: LiveHandlers | null = null
  private retry: ReturnType<typeof setTimeout> | undefined
  private helloTimer: ReturnType<typeof setTimeout> | undefined
  private polling: AbortController | null = null

  start(handlers: LiveHandlers): void {
    this.stop()
    this.handlers = handlers
    this.connect()
  }

  stop(): void {
    clearTimeout(this.retry)
    clearTimeout(this.helloTimer)
    this.source?.close()
    this.source = null
    this.polling?.abort()
    this.polling = null
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
    if (!this.handlers) return
    if (typeof EventSource === 'undefined' || pollingRemembered()) {
      this.poll()
      return
    }
    this.transport = 'stream'
    this.state = 'connecting'
    const es = new EventSource(`${somaBase()}/events`)
    this.source = es
    this.helloTimer = setTimeout(() => {
      if (this.source !== es) return
      rememberPolling()
      es.close()
      this.source = null
      this.poll()
    }, HELLO_TIMEOUT_MS)
    for (const name of EVENT_NAMES) {
      es.addEventListener(name, (e) => {
        if (this.source !== es) return
        const raw = (e as MessageEvent<string>).data
        let data: unknown
        try {
          data = JSON.parse(raw)
        } catch {
          if (name !== 'hello') return
          data = {} // an older server: no editing together
        }
        this.dispatch(name, data)
      })
    }
    es.onerror = () => {
      if (this.source !== es) return
      if (es.readyState === EventSource.CLOSED) {
        // Refused (401, 503 …): the browser gives up; try again later.
        clearTimeout(this.helloTimer)
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

  /** Long polling: one request after the other, each answered with the events since the last. */
  private poll(): void {
    this.polling?.abort()
    const ctrl = new AbortController()
    this.polling = ctrl
    this.transport = 'poll'
    this.state = 'connecting'
    void (async () => {
      let stream = ''
      let after = 0
      while (!ctrl.signal.aborted) {
        const query = stream ? `?stream=${encodeURIComponent(stream)}&after=${after}` : ''
        let res: Response
        try {
          res = await fetch(`${somaBase()}/poll${query}`, { cache: 'no-store', credentials: 'same-origin', signal: ctrl.signal })
        } catch {
          if (ctrl.signal.aborted) return
          this.state = 'connecting'
          await sleep(RETRY_MS, ctrl.signal)
          continue
        }
        if (ctrl.signal.aborted) return
        if (res.status === 410) {
          // The stream ended (expired, soma reopened): start a new one.
          stream = ''
          after = 0
          continue
        }
        if (!res.ok) {
          // Refused (signed out, no access, too many streams): try again later.
          this.state = 'offline'
          this.handlers?.refused()
          stream = ''
          after = 0
          await sleep(RETRY_MS, ctrl.signal)
          continue
        }
        let body: Polled
        try {
          body = (await res.json()) as Polled
        } catch {
          if (ctrl.signal.aborted) return
          await sleep(RETRY_MS, ctrl.signal)
          continue
        }
        if (ctrl.signal.aborted) return
        stream = body.stream
        for (const e of body.events) {
          after = Math.max(after, e.seq)
          this.dispatch(e.name, e.data)
        }
      }
    })()
  }

  private dispatch(name: string, data: unknown): void {
    const obj = (data && typeof data === 'object' ? data : {}) as Record<string, unknown>
    switch (name) {
      case 'hello': {
        clearTimeout(this.helloTimer)
        const stream = typeof obj.stream === 'string' ? obj.stream : ''
        this.stream = stream
        this.state = 'live'
        this.version++
        const waiting = this.waiters
        this.waiters = []
        for (const w of waiting) w(stream)
        this.handlers?.resync()
        return
      }
      case 'change':
        this.version++
        this.handlers?.change(data as Change)
        return
      case 'access':
        this.handlers?.access()
        return
      case 'trash':
        this.trashVersion++
        return
      case 'comments':
        if (typeof obj.path === 'string') this.commentsChanged = { path: obj.path, n: this.commentsChanged.n + 1 }
        return
      case 'notification':
        this.notificationVersion++
        return
      case 'banners':
        this.bannersVersion++
        return
      default:
        if ((COLLAB_EVENTS as string[]).includes(name)) this.handlers?.collab(name as CollabEventName, data)
    }
  }
}

export const live = new Live()
