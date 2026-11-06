import type { Change } from '../api/soma'

export type LiveState = 'connecting' | 'live' | 'offline'

export interface LiveHandlers {
  /** Stream (re)connected: reload what is shown, something may have been missed. */
  resync(): void
  /** Something changed in the soma. */
  change(c: Change): void
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
  }

  private connect(): void {
    if (!this.handlers || typeof EventSource === 'undefined') return
    this.state = 'connecting'
    const es = new EventSource('/api/soma/events')
    this.source = es
    es.addEventListener('hello', () => {
      this.state = 'live'
      this.version++
      this.handlers?.resync()
    })
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
