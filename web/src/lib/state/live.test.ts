import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../api/base', () => ({ somaBase: () => '/api/somas/s1' }))

import { HELLO_TIMEOUT_MS, live, type LiveHandlers } from './live.svelte'

// An EventSource that connects but never delivers anything, as behind a
// proxy that holds streams back; tests may emit events on it themselves.
class FakeEventSource {
  static CLOSED = 2
  static last: FakeEventSource | null = null
  readyState = 1
  closed = false
  onerror: (() => void) | null = null
  private listeners = new Map<string, ((e: { data: string }) => void)[]>()
  constructor(public url: string) {
    FakeEventSource.last = this
  }
  addEventListener(name: string, fn: (e: { data: string }) => void) {
    this.listeners.set(name, [...(this.listeners.get(name) ?? []), fn])
  }
  emit(name: string, data: unknown) {
    for (const fn of this.listeners.get(name) ?? []) fn({ data: JSON.stringify(data) })
  }
  close() {
    this.closed = true
  }
}

function handlers() {
  return { resync: vi.fn(), change: vi.fn(), access: vi.fn(), collab: vi.fn(), refused: vi.fn() } satisfies LiveHandlers
}

// A server answering polls from a script; a poll without an answer waits until aborted.
function fakeServer(answers: (url: string) => { status?: number; body?: unknown } | undefined) {
  const urls: string[] = []
  const fetch = vi.fn((url: string, init: RequestInit) => {
    urls.push(url)
    const a = answers(url)
    if (!a) {
      return new Promise<Response>((_, reject) => init.signal?.addEventListener('abort', () => reject(new Error('aborted'))))
    }
    return Promise.resolve(new Response(JSON.stringify(a.body ?? { error: 'x' }), { status: a.status ?? 200 }))
  })
  vi.stubGlobal('fetch', fetch)
  return urls
}

const flush = async () => {
  for (let i = 0; i < 10; i++) await Promise.resolve()
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('EventSource', FakeEventSource)
  const store = new Map<string, string>()
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, v),
    removeItem: (k: string) => void store.delete(k),
  })
})

afterEach(() => {
  live.stop()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('live connection', () => {
  it('uses the event stream when its hello arrives', async () => {
    const urls = fakeServer(() => undefined)
    const h = handlers()
    live.start(h)
    FakeEventSource.last!.emit('hello', { ready: true, stream: 'st1' })
    expect(live.state).toBe('live')
    expect(live.stream).toBe('st1')
    expect(live.transport).toBe('stream')
    await vi.advanceTimersByTimeAsync(HELLO_TIMEOUT_MS * 2)
    expect(urls).toEqual([])
    expect(h.resync).toHaveBeenCalledOnce()
  })

  it('polls when the stream is held back, and remembers it', async () => {
    let polls = 0
    const urls = fakeServer((url) => {
      polls++
      if (!url.includes('stream=')) return { body: { stream: 'p1', events: [{ seq: 0, name: 'hello', data: { ready: true, stream: 'p1' } }] } }
      if (polls === 2) return { body: { stream: 'p1', events: [{ seq: 1, name: 'change', data: { tree: true, notes: [], removed: [] } }] } }
      if (polls === 3) return { status: 410 }
      return undefined
    })
    const h = handlers()
    live.start(h)
    const es = FakeEventSource.last!
    expect(live.state).toBe('connecting')

    await vi.advanceTimersByTimeAsync(HELLO_TIMEOUT_MS)
    await flush()
    expect(es.closed).toBe(true)
    expect(live.transport).toBe('poll')
    expect(live.state).toBe('live')
    expect(live.stream).toBe('p1')
    expect(h.change).toHaveBeenCalledWith({ tree: true, notes: [], removed: [] })
    // Polls acknowledge what they got; a stream that ended is opened again.
    expect(urls.slice(0, 4)).toEqual([
      '/api/somas/s1/poll',
      '/api/somas/s1/poll?stream=p1&after=0',
      '/api/somas/s1/poll?stream=p1&after=1',
      '/api/somas/s1/poll',
    ])
    expect(h.resync).toHaveBeenCalledTimes(2)

    // The next start polls at once, without waiting for a stream first.
    live.stop()
    FakeEventSource.last = null
    live.start(handlers())
    await flush()
    expect(FakeEventSource.last).toBeNull()
    expect(live.transport).toBe('poll')
  })

  it('stops polling when the connection stops', async () => {
    vi.stubGlobal('EventSource', undefined)
    const urls = fakeServer(() => undefined)
    live.start(handlers())
    await flush()
    expect(urls).toEqual(['/api/somas/s1/poll'])
    live.stop()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(urls).toHaveLength(1)
    expect(live.state).toBe('offline')
  })
})
