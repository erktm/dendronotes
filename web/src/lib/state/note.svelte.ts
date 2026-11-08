import { getSyncedVersion, receiveUpdates, sendableUpdates } from '@codemirror/collab'
import { ChangeSet, Text } from '@codemirror/state'
import type { EditorView } from '@codemirror/view'
import { ApiError } from '../api/client'
import * as collab from '../api/collab'
import type { CollabEvent, NoticeEvent, Peer, PresenceEvent, SavedEvent, WireUpdate } from '../api/collab'
import { errorMessage } from '../errors'
import { isNote } from '../paths'
import { strings } from '../strings'

export type NoteStatus = 'loading' | 'ready' | 'missing' | 'unsupported' | 'too-large' | 'error'

export interface NoteHost {
  toast(text: string, kind?: 'error' | 'info'): void
  /** Whether the account may change notes in the open soma (not a viewer). */
  readonly canEdit: boolean
  /** The id of this tab's event stream (waits until it is connected). */
  stream(): Promise<string>
}

/** How long typing collects before it is sent. */
const PUSH_DELAY = 60
/** How often the cursor is shared at most. */
const PRESENCE_DELAY = 120

const toWire = (u: { clientID: string; changes: ChangeSet }): WireUpdate => ({ clientID: u.clientID, changes: u.changes.toJSON() })
const fromWire = (u: WireUpdate) => ({ clientID: u.clientID, changes: ChangeSet.fromJSON(u.changes) })

/**
 * One open note, edited together with everyone else who has it open. The
 * document lives in the editor (CodeMirror with @codemirror/collab, which
 * stays mounted while the note is open, also in the reading view); this
 * session talks to the server: it sends local changes for the version the
 * editor has seen, applies everyone's changes that arrive on the event
 * stream, catches up after gaps and rejoins after a reconnect. The server
 * writes the note to its file; nothing is saved from here.
 */
export class NoteSession {
  path = $state('')
  status = $state<NoteStatus>('loading')
  /** The text as the editor shows it (reading view, outline, word count). */
  content = $state('')
  /** The file cannot be written (a symbolic link). */
  fileReadOnly = $state(false)
  /** The others in this note. */
  peers = $state.raw<Peer[]>([])
  /** Bumped when the editor has to start over with a new document. */
  generation = $state(0)
  /** Local changes the server has not confirmed yet. */
  pending = $state(0)
  /** The version the editor is at, and the version written to the file. */
  synced = $state(0)
  savedVersion = $state(0)
  /** Sending failed and is being retried. */
  offline = $state(false)
  failed = $state(false)
  /** Put the cursor into the editor once loaded (e.g. for a new note). */
  autofocus = $state(false)

  /** Document instance on the server; events of other instances are ignored. */
  doc = ''
  /** Version the editor starts from (for a newly created editor). */
  startVersion = 0
  readonly clientID = Math.random().toString(36).slice(2, 12)
  /** Where the cursor and the scroll position were (restored after a restart of the editor). */
  lastSelection: { anchor: number; head: number } | null = null
  lastScroll = 0
  lastFocused = false

  private host: NoteHost
  private stream = ''
  private peerId = ''
  private view: EditorView | null = null
  /** The text at the synced version: the base for bringing back local changes. */
  private base: Text = Text.empty
  private pushTimer: ReturnType<typeof setTimeout> | undefined
  private presenceTimer: ReturnType<typeof setTimeout> | undefined
  private pushing: Promise<void> | null = null
  private pulling: Promise<void> | null = null
  private rejoining: Promise<void> | null = null
  private wantSave = false
  /** A push was accepted up to this version; its confirmation is on the way. */
  private awaiting = 0
  private closed = false

  constructor(path: string, host: NoteHost) {
    this.path = path
    this.host = host
  }

  /** Nothing can be changed: a symbolic link, or the soma is only shared for reading. */
  get readOnly(): boolean {
    return this.fileReadOnly || !this.host.canEdit
  }

  /**
   * Why nothing can be changed. The server marks notes read-only for viewers
   * too, so the role decides first; a symbolic link otherwise.
   */
  get readOnlyReason(): 'viewer' | 'file' | null {
    if (!this.host.canEdit) return 'viewer'
    return this.fileReadOnly ? 'file' : null
  }

  /** Changes exist that are not in the file yet. */
  get unsaved(): boolean {
    return this.pending > 0 || this.synced > this.savedVersion
  }

  async load(): Promise<void> {
    if (!isNote(this.path)) {
      this.status = 'unsupported'
      return
    }
    this.status = 'loading'
    try {
      const stream = await this.host.stream()
      const j = await collab.join(this.path, stream)
      if (this.closed) {
        void collab.leave(this.path, stream).catch(() => {})
        return
      }
      this.accept(j, stream)
      this.status = 'ready'
    } catch (err) {
      if (this.closed) return
      if (err instanceof ApiError && err.status === 404) this.status = 'missing'
      else if (err instanceof ApiError && err.code === 'not_a_note') this.status = 'unsupported'
      else if (err instanceof ApiError && err.code === 'too_large') this.status = 'too-large'
      else this.status = 'error'
    }
  }

  private accept(j: collab.Joined, stream: string): void {
    this.awaiting = 0
    this.stream = stream
    this.doc = j.doc
    this.startVersion = j.version
    this.synced = j.version
    this.savedVersion = j.saved
    this.content = j.text
    this.base = Text.of(j.text.split('\n'))
    this.fileReadOnly = j.readOnly
    this.peerId = j.peer
    this.peers = j.peers.filter((p) => p.id !== j.peer)
    this.pending = 0
    this.offline = false
    this.failed = false
  }

  // --- The editor ----------------------------------------------------------

  /** The editor for this note is ready (or was created again). */
  attach(view: EditorView): void {
    this.view = view
    // Updates that arrived before the editor existed were skipped.
    void this.pull()
  }

  detach(view: EditorView): void {
    if (this.view !== view) return
    this.lastSelection = { anchor: view.state.selection.main.anchor, head: view.state.selection.main.head }
    this.lastScroll = view.scrollDOM.scrollTop
    this.lastFocused = view.hasFocus
    this.view = null
  }

  /** The document in the editor changed (typing, or updates received). */
  changed(view: EditorView): void {
    if (view !== this.view) return
    this.content = view.state.doc.toString()
    this.pending = sendableUpdates(view.state).length
    if (this.pending > 0) this.schedulePush()
  }

  /** The cursor moved: tell the others (shortly, and not too often). */
  selectionChanged(view: EditorView): void {
    if (view !== this.view || this.closed) return
    clearTimeout(this.presenceTimer)
    this.presenceTimer = setTimeout(() => {
      const v = this.view
      if (!v || this.closed || this.status !== 'ready') return
      const { anchor, head } = v.state.selection.main
      collab.presence(this.path, this.stream, this.doc, v.hasFocus ? { anchor, head } : null).catch(() => {})
    }, PRESENCE_DELAY)
  }

  /** Ticks or unticks the task on a 0-based line (reading view). */
  toggleTask(line: number): void {
    const view = this.view
    if (!view || this.readOnly || line < 0 || line >= view.state.doc.lines) return
    const l = view.state.doc.line(line + 1)
    const m = /\[( |x|X)\]/.exec(l.text)
    if (!m) return
    const at = l.from + m.index + 1
    view.dispatch({ changes: { from: at, to: at + 1, insert: m[1] === ' ' ? 'x' : ' ' }, userEvent: 'input' })
  }

  // --- Sending ---------------------------------------------------------------

  private schedulePush(delay = PUSH_DELAY): void {
    if (this.readOnly || this.closed) return
    clearTimeout(this.pushTimer)
    this.pushTimer = setTimeout(() => void this.push(), delay)
  }

  /** Sends local changes now; with save, the server writes the file at once (⌘S). */
  async push(save = false): Promise<void> {
    clearTimeout(this.pushTimer)
    if (save) this.wantSave = true
    if (this.pushing) await this.pushing
    const view = this.view
    if (!view || this.closed || this.readOnly || this.status !== 'ready') return
    const updates = sendableUpdates(view.state)
    if (updates.length === 0 && !this.wantSave) return
    const version = getSyncedVersion(view.state)
    // Sent already and not confirmed yet: sending them again would only be
    // refused. apply() sends the rest once the confirmation is in.
    if (version < this.awaiting) return
    const saveNow = this.wantSave
    this.wantSave = false
    this.pushing = (async () => {
      try {
        const r = await collab.push(this.path, this.stream, this.doc, version, updates.map(toWire), saveNow)
        this.offline = false
        this.failed = false
        this.awaiting = r.version
        // The confirmation comes on the event stream; should it be late, ask.
        setTimeout(() => {
          if (this.view && getSyncedVersion(this.view.state) < r.version) void this.pull()
        }, 600)
      } catch (err) {
        this.handlePushError(err, saveNow)
      } finally {
        this.pushing = null
      }
    })()
    await this.pushing
  }

  private handlePushError(err: unknown, save: boolean): void {
    if (this.closed) return
    if (save) this.wantSave = true
    if (!(err instanceof ApiError)) {
      this.failed = true
      return
    }
    switch (true) {
      case err.code === 'version_mismatch':
        // Others were faster: catch up (our changes are rebased), then send again.
        void this.pull().then(() => this.schedulePush(0))
        return
      case err.code === 'resync' || err.code === 'unknown_stream':
        void this.rejoin()
        return
      case err.status === 0 || err.status === 503 || err.status === 429:
        this.offline = err.status === 0
        this.schedulePush(err.status === 429 ? 500 : 2000)
        return
      case err.status === 403:
        return // no longer allowed to edit (became a reader); the editor locks itself
      default:
        this.failed = true
        this.host.toast(strings.toasts.saveFailed(errorMessage(err)))
    }
  }

  /** Sends pending changes (before switching notes or moving files). */
  async flush(): Promise<void> {
    if (this.pending > 0 || this.pushing) await this.push()
  }

  // --- Receiving -------------------------------------------------------------

  /** Updates from the event stream. */
  receive(e: CollabEvent): void {
    const view = this.view
    if (e.doc !== this.doc || !view || this.closed) return
    const v = getSyncedVersion(view.state)
    if (e.from > v) {
      void this.pull() // missed something
      return
    }
    const fresh = e.updates.slice(v - e.from)
    if (fresh.length) this.apply(fresh)
  }

  private apply(updates: WireUpdate[]): void {
    const view = this.view
    if (!view) return
    const parsed = updates.map(fromWire)
    for (const u of parsed) this.base = u.changes.apply(this.base)
    view.dispatch(receiveUpdates(view.state, parsed))
    this.synced = getSyncedVersion(view.state)
    this.pending = sendableUpdates(view.state).length
    this.content = view.state.doc.toString()
    if (this.pending > 0 && this.synced >= this.awaiting) this.schedulePush()
  }

  /** Catches up with the server after a gap. */
  pull(): Promise<void> {
    this.pulling ??= (async () => {
      try {
        const view = this.view
        if (!view || this.closed || this.status !== 'ready') return
        const from = getSyncedVersion(view.state)
        const r = await collab.pull(this.path, this.doc, from)
        if (this.view !== view || getSyncedVersion(view.state) !== from) return
        if (r.updates.length) this.apply(r.updates)
      } catch (err) {
        if (err instanceof ApiError && err.code === 'resync') void this.rejoin()
      } finally {
        this.pulling = null
      }
    })()
    return this.pulling
  }

  /**
   * Joins again with the current stream (after a reconnect, or when the
   * server lost the document). Local changes the server never confirmed are
   * sent along and merged in.
   */
  rejoin(): Promise<void> {
    this.rejoining ??= (async () => {
      try {
        if (this.closed || this.status !== 'ready') return
        const stream = await this.host.stream()
        const view = this.view
        const mine = view ? view.state.doc.toString() : this.content
        const base = this.base.toString()
        const j = await collab.join(this.path, stream, mine !== base ? { doc: this.doc, base, mine } : undefined)
        if (this.closed) return
        if (j.doc === this.doc && view) {
          // The same document: catch up and send what is still open.
          this.stream = stream
          this.peerId = j.peer
          this.peers = j.peers.filter((p) => p.id !== j.peer)
          this.savedVersion = Math.max(this.savedVersion, j.saved)
          this.fileReadOnly = j.readOnly
          await this.pull()
          if (sendableUpdates(view.state).length) this.schedulePush(0)
          return
        }
        if (view) this.detach(view)
        this.accept(j, stream)
        this.generation++ // a new editor for the new document
        if (j.conflict) this.host.toast(strings.collab.conflict, 'info')
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) this.status = 'missing'
        else if (err instanceof ApiError && err.status === 0) this.offline = true
      } finally {
        this.rejoining = null
      }
    })()
    return this.rejoining
  }

  presence(e: PresenceEvent): void {
    if (e.doc === this.doc) this.peers = e.peers.filter((p) => p.id !== this.peerId)
  }

  saved(e: SavedEvent): void {
    if (e.doc !== this.doc) return
    this.savedVersion = Math.max(this.savedVersion, e.version)
    this.failed = false
  }

  /** Something happened to the note; returns the new path after a rename. */
  notice(e: NoticeEvent): string | null {
    if (e.doc !== this.doc) return null
    switch (e.kind) {
      case 'renamed':
        if (e.to) this.path = e.to
        return e.to ?? null
      case 'removed':
        this.status = 'missing'
        break
      case 'merged':
        this.host.toast(strings.collab.merged, 'info')
        break
      case 'conflict':
        this.host.toast(strings.collab.conflict, 'info')
        break
      case 'save_failed':
        this.failed = true
        break
      case 'read_only':
        this.fileReadOnly = true
        break
    }
    return null
  }

  // --- Lifecycle -----------------------------------------------------------------

  close(): void {
    if (this.closed) return
    this.closed = true
    clearTimeout(this.pushTimer)
    clearTimeout(this.presenceTimer)
    if (this.stream && this.doc) void collab.leave(this.path, this.stream).catch(() => {})
  }

  /** The note changed on disk (event stream): only matters if it was missing. */
  async changedOnDisk(): Promise<void> {
    if (this.status === 'missing' && !this.closed) await this.load()
  }

  /** After reconnecting: join again with the new stream. */
  async revalidate(): Promise<void> {
    if (this.closed) return
    if (this.status === 'missing' || this.status === 'error') await this.load()
    else if (this.status === 'ready') await this.rejoin()
  }
}
