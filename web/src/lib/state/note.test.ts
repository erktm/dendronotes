import { afterEach, describe, expect, it, vi } from 'vitest'
import { collab } from '@codemirror/collab'
import { ChangeSet, EditorState, Text, Transaction, type TransactionSpec } from '@codemirror/state'
import type { EditorView } from '@codemirror/view'
import { ApiError } from '../api/client'
import type { CollabEvent, WireUpdate } from '../api/collab'

// An in-memory server with the rules of internal/collab: pushes only on the
// current version, every update goes to everyone (in order, sometimes late,
// sometimes lost), pull catches up.
const server = vi.hoisted(() => ({
  doc: 'd1',
  text: '',
  version: 0,
  recovered: null as null | { doc: string; base: string; mine: string },
  updates: [] as WireUpdate[],
  listeners: new Map<string, (e: CollabEvent) => void>(),
  dropEvery: 0,
  sent: 0,
}))

vi.mock('../api/collab', async () => {
  const { ChangeSet: CS, Text: T } = await import('@codemirror/state')
  const { ApiError: Err } = await import('../api/client')
  const wait = () => new Promise((r) => setTimeout(r, Math.random() * 15))
  const queues = new Map<string, Promise<void>>()
  return {
    join: async (_path: string, stream: string, recover?: { doc: string; base: string; mine: string }) => {
      if (recover && recover.doc !== server.doc) {
        server.recovered = recover
        server.text = recover.mine // nobody else changed it meanwhile
      }
      return {
      doc: server.doc,
      version: server.version,
      text: server.text,
      readOnly: false,
      saved: 0,
      peer: stream,
      peers: [],
      }
    },
    leave: async () => ({ ok: true }),
    presence: async () => ({ ok: true }),
    pull: async (_p: string, _d: string, version: number) => {
      await wait()
      return { updates: server.updates.slice(version), version: server.version }
    },
    push: async (path: string, _s: string, doc: string, version: number, updates: WireUpdate[]) => {
      await wait()
      if (doc !== server.doc) throw new Err(410, 'resync')
      if (version !== server.version) throw new Err(409, 'version_mismatch', { version: server.version })
      let text = T.of(server.text.split('\n'))
      for (const u of updates) text = CS.fromJSON(u.changes).apply(text)
      server.text = text.toString()
      const from = server.version
      server.updates.push(...updates)
      server.version += updates.length
      const e = { path, doc, from, updates }
      for (const [id, listener] of server.listeners) {
        // Delivered in order per stream, after a random delay; now and then lost.
        const lost = server.dropEvery > 0 && ++server.sent % server.dropEvery === 0
        const q = (queues.get(id) ?? Promise.resolve()).then(wait).then(() => {
          if (!lost) listener(e)
        })
        queues.set(id, q)
      }
      return { version: server.version }
    },
  }
})

const { NoteSession } = await import('./note.svelte')

/** Just enough of an EditorView: state, dispatch and the editor's listener. */
class FakeView {
  state: EditorState
  hasFocus = false
  scrollDOM = { scrollTop: 0 }
  onChange: () => void = () => {}

  constructor(doc: string, startVersion: number, clientID: string) {
    this.state = EditorState.create({ doc, extensions: [collab({ startVersion, clientID })] })
  }

  dispatch(tr: Transaction | TransactionSpec): void {
    const t = tr instanceof Transaction ? tr : this.state.update(tr)
    this.state = t.state
    if (t.docChanged) this.onChange()
  }
}

async function open(stream: string) {
  const host = { toast: () => {}, canEdit: true, stream: async () => stream }
  const s = new NoteSession('Shared.md', host)
  await s.load()
  const view = new FakeView(s.content, s.startVersion, s.clientID)
  const v = view as unknown as EditorView
  view.onChange = () => s.changed(v)
  s.attach(v)
  server.listeners.set(stream, (e) => s.receive(e))
  return { s, view }
}

const settle = async (check: () => boolean) => {
  for (let i = 0; i < 300 && !check(); i++) await new Promise((r) => setTimeout(r, 20))
}

afterEach(() => {
  server.listeners.clear()
})

describe('NoteSession (editing together)', () => {
  it('converges when several people type at the same time', async () => {
    Object.assign(server, { doc: 'd1', text: 'start\n', version: 0, updates: [], dropEvery: 7, sent: 0 })
    const tabs = await Promise.all(['a', 'b', 'c'].map(open))
    // Editors never split a character made of two UTF-16 units.
    const whole = (doc: Text, pos: number) => (pos > 0 && /[\ud800-\udbff]/.test(doc.sliceString(pos - 1, pos)) ? pos + 1 : pos)
    for (let round = 0; round < 25; round++) {
      for (const { view } of tabs) {
        const len = view.state.doc.length
        const from = whole(view.state.doc, Math.floor(Math.random() * (len + 1)))
        const to = whole(view.state.doc, Math.min(len, from + Math.floor(Math.random() * 3)))
        view.dispatch({ changes: { from, to, insert: ['x', 'ü', '😀', '\n', ''][round % 5]! }, userEvent: 'input' })
      }
      await new Promise((r) => setTimeout(r, Math.random() * 30))
    }
    // Lost events show up as gaps when the next one arrives; whatever is
    // missing at the end is fetched as after a reconnect.
    await settle(() => tabs.every(({ s }) => s.pending === 0))
    await Promise.all(tabs.map(({ s }) => s.pull()))
    await settle(() => tabs.every(({ s }) => s.pending === 0 && s.synced === server.version))
    for (const { s, view } of tabs) {
      expect(s.pending).toBe(0)
      expect(view.state.doc.toString()).toBe(server.text)
      expect(s.content).toBe(server.text)
    }
    expect(server.version).toBeGreaterThanOrEqual(25)
  }, 20000)

  it('ticks tasks through the editor document', async () => {
    Object.assign(server, { doc: 'd1', text: '- [ ] seeds\n- [x] water', version: 0, updates: [], dropEvery: 0, sent: 0 })
    const { s, view } = await open('t')
    s.toggleTask(0)
    s.toggleTask(1)
    await settle(() => s.pending === 0 && s.synced === server.version)
    expect(server.text).toBe('- [x] seeds\n- [ ] water')
    expect(view.state.doc.toString()).toBe(server.text)
  })

  it('brings local changes back when the server lost the document', async () => {
    Object.assign(server, { doc: 'd1', text: 'one', version: 0, updates: [], dropEvery: 0, sent: 0, recovered: null })
    const { s, view } = await open('r')
    // The server restarts (a new document) while this tab is typing.
    server.doc = 'd2'
    view.dispatch({ changes: { from: 3, insert: ' two' }, userEvent: 'input' })
    await settle(() => s.generation > 0)
    expect(server.recovered).toEqual({ doc: 'd1', base: 'one', mine: 'one two' })
    expect(s.doc).toBe('d2')
    expect(s.content).toBe('one two')
    expect(s.pending).toBe(0)
  })
})
