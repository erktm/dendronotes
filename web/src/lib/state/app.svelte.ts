import { SvelteSet } from 'svelte/reactivity'
import { ApiError, onUnauthorized } from '../api/client'
import * as api from '../api/soma'
import type { Change, Entry, Me } from '../api/soma'
import { errorMessage } from '../errors'
import {
  ancestors,
  basename,
  dirname,
  displayName,
  extname,
  isInside,
  isNote,
  join,
  nameProblem,
  withNoteExtension,
} from '../paths'
import { router } from '../router.svelte'
import { strings } from '../strings'
import { newNotePath, normalizePath, resolveSynapse } from '../synapses'
import { buildTree } from '../tree'
import { NoteSession } from './note.svelte'
import { live } from './live.svelte'
import { settings } from './settings.svelte'

type AuthState = 'checking' | 'anonymous' | 'authenticated'

/** An inline input in the file tree. */
export type TreeEdit = { kind: 'new-note' | 'new-folder'; parent: string } | { kind: 'rename'; path: string }

export interface Toast {
  id: number
  text: string
  kind: 'error' | 'info'
}

export interface ConfirmRequest {
  title: string
  message: string
  confirmLabel: string
  danger?: boolean
  resolve: (ok: boolean) => void
}

const EXPANDED_KEY = 'dendronotes.expanded'

function loadExpanded(): string[] {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(EXPANDED_KEY) ?? '[]')
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
  } catch {
    return []
  }
}

class App {
  auth = $state<AuthState>('checking')
  mode = $state<'password' | 'proxy'>('password')
  user = $state('')
  logoutUrl = $state('')

  entries = $state.raw<Entry[]>([])
  tree = $derived(buildTree(this.entries))
  treeLoaded = $state(false)
  expanded = new SvelteSet<string>(loadExpanded())
  edit = $state<TreeEdit | null>(null)

  /** Paths of all notes in the soma (for synapses and the quick switcher). */
  notePaths = $derived(this.entries.filter((e) => e.type === 'file' && isNote(e.path)).map((e) => e.path))

  note = $state.raw<NoteSession | null>(null)
  /** A note that should get the cursor when it opens. */
  private focusNext: string | null = null

  /** What the sidebar shows. */
  sidebarView = $state<'files' | 'search'>('files')
  searchQuery = $state('')
  /** Bumped to move the focus into the search field. */
  searchFocus = $state(0)

  switcherOpen = $state(false)
  settingsOpen = $state(false)
  /** A heading the open view should scroll to (set by the outline). */
  scrollRequest = $state.raw<{ line: number; slug: string } | null>(null)

  toasts = $state<Toast[]>([])
  confirmRequest = $state.raw<ConfirmRequest | null>(null)
  private toastId = 0
  /** A move started here is in progress: its "removed" events are expected. */
  private moving = 0
  private refreshTimer: ReturnType<typeof setTimeout> | undefined

  constructor() {
    onUnauthorized(() => this.sessionEnded())
  }

  // --- Authentication ---------------------------------------------------

  async init(): Promise<void> {
    try {
      this.signedIn(await api.me())
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        this.mode = err.body.mode === 'proxy' ? 'proxy' : 'password'
      } else {
        this.toast(errorMessage(err))
      }
      this.auth = 'anonymous'
    }
  }

  async login(password: string): Promise<void> {
    this.signedIn(await api.login(password))
  }

  async logout(): Promise<void> {
    await this.note?.flush()
    let logoutUrl = this.logoutUrl
    try {
      logoutUrl = (await api.logout()).logoutUrl || logoutUrl
    } catch {
      // Logging out locally is what matters.
    }
    live.stop()
    this.closeNote()
    router.go(null, true)
    this.entries = []
    this.treeLoaded = false
    this.auth = 'anonymous'
    if (this.mode === 'proxy' && logoutUrl) window.location.assign(logoutUrl)
  }

  persistExpanded(): void {
    try {
      localStorage.setItem(EXPANDED_KEY, JSON.stringify([...this.expanded]))
    } catch {
      // Private mode or storage disabled: the tree just forgets its state.
    }
  }

  private signedIn(me: Me): void {
    this.user = me.user
    this.mode = me.mode
    this.logoutUrl = me.logoutUrl ?? ''
    this.auth = 'authenticated'
    void this.refresh()
    live.start({
      resync: () => {
        void this.refresh()
        void this.note?.revalidate()
      },
      change: (c) => this.applyChange(c),
      refused: () => void this.checkSession(),
    })
  }

  private sessionEnded(): void {
    if (this.auth !== 'authenticated') return
    live.stop()
    this.auth = 'anonymous'
    this.toast(strings.auth.sessionEnded, 'info')
  }

  /** The event stream was refused: find out whether we are still signed in. */
  private async checkSession(): Promise<void> {
    try {
      await api.me()
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) this.sessionEnded()
    }
  }

  /** Applies a change reported by the server (made here or anywhere else). */
  private applyChange(c: Change): void {
    if (c.tree) {
      clearTimeout(this.refreshTimer)
      this.refreshTimer = setTimeout(() => void this.refresh(), 120)
    }
    const n = this.note
    if (!n) return
    const mine = c.notes.find((x) => x.path === n.path)
    if (mine) void n.changedOnDisk(mine.etag)
    if (c.removed.includes(n.path) && this.moving === 0) n.removedOnDisk()
  }

  // --- Tree -------------------------------------------------------------

  async refresh(): Promise<void> {
    try {
      this.entries = await api.getTree()
      this.treeLoaded = true
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 401)) this.toast(errorMessage(err))
    }
  }

  /** Shows the inline name input for a new note or folder in parent. */
  startCreate(kind: 'new-note' | 'new-folder', parent: string): void {
    if (parent) this.expanded.add(parent)
    this.edit = { kind, parent }
  }

  async createEntry(kind: 'note' | 'folder', parent: string, rawName: string): Promise<boolean> {
    const problem = nameProblem(rawName)
    if (problem) {
      this.toast(problem)
      return false
    }
    let name = rawName.normalize('NFC')
    if (kind === 'note') name = withNoteExtension(name)
    const path = join(parent, name)
    try {
      if (kind === 'note') await api.createNote(path)
      else await api.createFolder(path)
    } catch (err) {
      this.toast(errorMessage(err))
      return false
    }
    if (parent) this.expanded.add(parent)
    await this.refresh()
    if (kind === 'note') {
      this.focusNext = path
      await this.navigate(path)
    } else {
      this.expanded.add(path)
    }
    return true
  }

  async rename(path: string, rawName: string): Promise<boolean> {
    const problem = nameProblem(rawName)
    if (problem) {
      this.toast(problem)
      return false
    }
    let name = rawName.normalize('NFC')
    const entry = this.entries.find((e) => e.path === path)
    // Keep a note a note: "Plan" for "Idea.md" becomes "Plan.md".
    if (entry?.type === 'file' && isNote(path) && !isNote(name)) name += extname(path)
    const to = join(dirname(path), name)
    if (to === path) return true
    return this.moveTo(path, to)
  }

  async moveInto(path: string, targetDir: string): Promise<void> {
    if (isInside(targetDir, path)) {
      if (targetDir !== path) this.toast(strings.tree.moveIntoSelf)
      return
    }
    const to = join(targetDir, basename(path))
    if (to === path) return
    if (await this.moveTo(path, to)) {
      if (targetDir) this.expanded.add(targetDir)
    }
  }

  private async moveTo(from: string, to: string): Promise<boolean> {
    // Save first: the open note may be moved, or contain synapses to rewrite.
    await this.note?.flush()
    let res: api.MoveResult
    this.moving++
    try {
      res = await api.move(from, to, settings.updateSynapses)
    } catch (err) {
      this.toast(errorMessage(err))
      await this.refresh()
      return false
    } finally {
      this.moving--
    }
    const moved = res.path
    if (res.updatedSynapses) this.toast(strings.toasts.synapsesUpdated(res.updatedSynapses, res.updatedNotes ?? 0), 'info')
    if (res.failed?.length) this.toast(strings.toasts.synapsesSkipped(res.failed.length))
    // Carry expanded folders and the open note over to the new location.
    for (const p of [...this.expanded]) {
      if (isInside(p, from)) {
        this.expanded.delete(p)
        this.expanded.add(moved + p.slice(from.length))
      }
    }
    if (this.note && isInside(this.note.path, from)) {
      const next = moved + this.note.path.slice(from.length)
      this.note.path = next
      router.go(next, true)
    }
    await this.refresh()
    // Its own synapses may have been rewritten (relative links).
    void this.note?.revalidate()
    return true
  }

  // --- Search -------------------------------------------------------------

  /** Opens the search in the sidebar, optionally with a query. */
  openSearch(query?: string): void {
    if (query !== undefined) this.searchQuery = query
    this.sidebarView = 'search'
    this.searchFocus++
  }

  /** Opens a note and scrolls to a line (search results, backlinks). */
  async openAt(path: string, line: number): Promise<void> {
    await this.navigate(path)
    if (line >= 0 && this.note?.path === path) this.scrollRequest = { line, slug: '' }
  }

  async remove(path: string): Promise<void> {
    const isDir = this.entries.find((e) => e.path === path)?.type === 'dir'
    const ok = await this.ask({
      title: isDir ? strings.dialogs.deleteFolderTitle : strings.dialogs.deleteNoteTitle,
      message: strings.dialogs.deleteMessage(displayName(path), isDir),
      confirmLabel: strings.dialogs.moveToTrash,
      danger: true,
    })
    if (!ok) return
    try {
      await api.trash(path)
    } catch (err) {
      this.toast(errorMessage(err))
      await this.refresh()
      return
    }
    if (this.note && isInside(this.note.path, path)) {
      this.closeNote()
      router.go(null, true)
    }
    for (const p of [...this.expanded]) if (isInside(p, path)) this.expanded.delete(p)
    await this.refresh()
    this.toast(strings.toasts.movedToTrash(displayName(path)), 'info')
  }

  // --- Navigation -------------------------------------------------------

  /** User-initiated navigation (asks before throwing away a conflict). */
  async navigate(path: string | null): Promise<void> {
    const cur = this.note
    if (cur && cur.path !== path && cur.conflict !== null) {
      const ok = await this.ask({
        title: strings.dialogs.discardTitle,
        message: strings.dialogs.discardMessage(displayName(cur.path)),
        confirmLabel: strings.dialogs.discard,
        danger: true,
      })
      if (!ok) return
    }
    router.go(path)
  }

  // --- Synapses ----------------------------------------------------------

  /** The note a synapse in fromPath points to, or null if it does not exist yet. */
  resolveSynapse(target: string, markdown: boolean, fromPath: string): string | null {
    return resolveSynapse({ target, markdown }, fromPath, this.notePaths)
  }

  /** Follows a synapse; an unresolved one creates its note first. */
  async openSynapse(target: string, markdown: boolean, fromPath: string): Promise<void> {
    const resolved = this.resolveSynapse(target, markdown, fromPath)
    if (resolved) {
      await this.navigate(resolved)
      return
    }
    const path = markdown ? normalizePath(join(dirname(fromPath), target)) : newNotePath(target, fromPath)
    if (!path) {
      this.toast(strings.errors.byCode.invalid_path!)
      return
    }
    const problem = nameProblem(basename(path))
    if (problem) {
      this.toast(problem)
      return
    }
    try {
      await api.createNote(path)
    } catch (err) {
      if (!(err instanceof ApiError && err.code === 'exists')) {
        this.toast(errorMessage(err))
        return
      }
    }
    for (const dir of ancestors(path)) this.expanded.add(dir)
    await this.refresh()
    this.focusNext = path
    await this.navigate(path)
  }

  /** Follows the URL: opens the note it names (or none). */
  openPath(path: string | null): void {
    if (this.note?.path === path) return
    const prev = this.note
    if (prev) void prev.flush().finally(() => prev.close())
    if (path === null) {
      this.note = null
      return
    }
    const session = new NoteSession(path, this)
    session.autofocus = this.focusNext === path
    this.focusNext = null
    this.note = session
    void session.load()
    for (const dir of ancestors(path)) this.expanded.add(dir)
  }

  private closeNote(): void {
    this.note?.close()
    this.note = null
  }

  // --- Dialogs and notifications -----------------------------------------

  ask(req: Omit<ConfirmRequest, 'resolve'>): Promise<boolean> {
    this.confirmRequest?.resolve(false)
    return new Promise((resolve) => {
      this.confirmRequest = { ...req, resolve }
    })
  }

  answer(ok: boolean): void {
    const req = this.confirmRequest
    this.confirmRequest = null
    req?.resolve(ok)
  }

  toast(text: string, kind: 'error' | 'info' = 'error'): void {
    const id = ++this.toastId
    this.toasts.push({ id, text, kind })
    setTimeout(() => this.dismiss(id), kind === 'error' ? 7000 : 4000)
  }

  dismiss(id: number): void {
    this.toasts = this.toasts.filter((t) => t.id !== id)
  }
}

export const app = new App()
