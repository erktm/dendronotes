import { SvelteSet } from 'svelte/reactivity'
import { ApiError, onUnauthorized } from '../api/client'
import * as api from '../api/soma'
import type { Change, Entry, Me, Membership, User } from '../api/soma'
import { currentSoma, setSoma } from '../api/base'
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
import { newNotePath, normalizePath, resolveFile, resolveSynapse } from '../synapses'
import { type FolderOrder, type Orders, type SortMode, type TreeNode, buildTree, outermost, reorder, visiblePaths } from '../tree'
import { readDropped } from '../dropped'
import { isDrawingPath, withDrawingExtension } from '../drawing/paths'
import * as drawingsApi from '../api/drawings'
import * as linksApi from '../api/links'
import { NoteSession } from './note.svelte'
import { live, type CollabEventName } from './live.svelte'
import type { CollabEvent, NoticeEvent, PresenceEvent, SavedEvent } from '../api/collab'
import { settings } from './settings.svelte'
import { capabilities, type Capability } from '../roles'

type AuthState = 'checking' | 'anonymous' | 'authenticated'

/** An inline input in the file tree. */
export type NewEntryKind = 'new-note' | 'new-folder' | 'new-drawing'
export type TreeEdit = { kind: NewEntryKind; parent: string } | { kind: 'rename'; path: string }

/** What a tree input creates, for its edit kind. */
export function entryKind(kind: NewEntryKind): 'note' | 'folder' | 'drawing' {
  return kind === 'new-folder' ? 'folder' : kind === 'new-drawing' ? 'drawing' : 'note'
}

export interface Toast {
  id: number
  text: string
  kind: 'error' | 'info'
}

export interface PromptRequest {
  title: string
  text?: string
  label: string
  value: string
  placeholder?: string
  confirmLabel: string
  resolve: (value: string | null) => void
}

export interface ConfirmRequest {
  title: string
  message: string
  confirmLabel: string
  danger?: boolean
  resolve: (ok: boolean) => void
}

const EXPANDED_KEY = 'dendronotes.expanded'
const LAST_SOMA_KEY = 'dendronotes.lastSoma'

/** Open folders are remembered per soma. */
function loadExpanded(soma: string): string[] {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(`${EXPANDED_KEY}.${soma}`) ?? '[]')
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
  } catch {
    return []
  }
}

function storage(key: string, value?: string): string | null {
  try {
    if (value !== undefined) localStorage.setItem(key, value)
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

/** Most files one drop from the computer uploads. */
const MAX_DROP_FILES = 1000

class App {
  auth = $state<AuthState>('checking')
  mode = $state<'local' | 'proxy'>('local')
  user = $state.raw<User | null>(null)
  logoutUrl = $state('')
  /** E-mail is configured on the server (the profile offers its settings). */
  mailEnabled = $state(false)
  isAdmin = $derived(this.user?.role === 'admin')

  /** The somas the account can open, and the one that is open. */
  somas = $state.raw<Membership[]>([])
  somasLoaded = $state(false)
  somaId = $state('')
  soma = $derived(this.somas.find((s) => s.id === this.somaId) ?? null)
  /** What the account may do in the open soma (lib/roles.ts). */
  can = $derived(capabilities(this.soma?.role))
  /** The account may change notes in the open soma (NoteHost). */
  canEdit = $derived(this.can.write)

  entries = $state.raw<Entry[]>([])
  /** How folders are sorted, for everyone with access. */
  orders = $state.raw<Orders>({})
  tree = $derived(buildTree(this.entries, this.orders))
  /** Entries picked with ⌘/Ctrl- or Shift-click, to move or delete together. */
  selected = new SvelteSet<string>()
  private selectAnchor: string | null = null
  treeLoaded = $state(false)
  expanded = new SvelteSet<string>()
  edit = $state<TreeEdit | null>(null)

  /** Paths of all notes in the soma (for synapses and the quick switcher). */
  notePaths = $derived(this.entries.filter((e) => e.type === 'file' && isNote(e.path)).map((e) => e.path))
  /** Paths of all files (for attachments). */
  filePaths = $derived(this.entries.filter((e) => e.type === 'file').map((e) => e.path))

  note = $state.raw<NoteSession | null>(null)
  /** The note opened last (the center of the local graph). */
  lastNotePath = $state<string | null>(null)
  /** A note that should get the cursor when it opens. */
  private focusNext: string | null = null

  /** What the sidebar shows. */
  sidebarView = $state<'files' | 'search'>('files')
  searchQuery = $state('')
  /** Bumped to move the focus into the search field. */
  searchFocus = $state(0)

  switcherOpen = $state(false)
  settingsOpen = $state(false)
  profileOpen = $state(false)
  shareOpen = $state(false)
  trashOpen = $state(false)
  /** The comments panel is shown next to the note. */
  commentsOpen = $state(false)
  /** The history of the open note is shown. */
  historyOpen = $state(false)
  /** A heading the open view should scroll to (set by the outline). */
  scrollRequest = $state.raw<{ line: number; slug: string } | null>(null)

  toasts = $state<Toast[]>([])
  confirmRequest = $state.raw<ConfirmRequest | null>(null)
  promptRequest = $state.raw<PromptRequest | null>(null)
  private toastId = 0
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
        this.mode = err.body.mode === 'proxy' ? 'proxy' : 'local'
      } else {
        this.toast(errorMessage(err))
      }
      this.auth = 'anonymous'
    }
  }

  async login(username: string, password: string): Promise<void> {
    this.signedIn(await api.login(username, password))
  }

  /** Signs in with a confirmed sign-up (the confirmation page). */
  async confirmSignup(token: string): Promise<void> {
    this.signedIn(await api.confirmSignup(token))
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
    this.entries = []
    this.treeLoaded = false
    this.somas = []
    this.somasLoaded = false
    this.somaId = ''
    setSoma('')
    this.user = null
    this.auth = 'anonymous'
    // Sign out at the proxy or the OpenID Connect provider too.
    if (logoutUrl) window.location.assign(logoutUrl)
  }

  persistExpanded(): void {
    if (this.somaId) storage(`${EXPANDED_KEY}.${this.somaId}`, JSON.stringify([...this.expanded]))
  }

  private signedIn(me: Me): void {
    this.user = me.user
    this.mode = me.mode
    this.logoutUrl = me.logoutUrl ?? ''
    this.mailEnabled = me.mail ?? false
    this.auth = 'authenticated'
    void this.loadSomas()
  }

  /** Updates the signed-in account after a profile change. */
  setUser(u: User): void {
    this.user = u
  }

  // --- Somas -------------------------------------------------------------

  /** Loads the account's somas; without a soma in the URL, opens the last one. */
  async loadSomas(): Promise<void> {
    try {
      this.somas = await api.listSomas()
      this.somasLoaded = true
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 401)) this.toast(errorMessage(err))
      return
    }
    const wanted = router.soma
    if (wanted && this.somas.some((s) => s.id === wanted)) return // the shell opens it
    if (router.view === 'admin' || router.view === 'tutorials') return
    if (wanted) this.toast(strings.somas.notFound)
    router.goSoma(this.preferredSoma(), true)
  }

  /** The soma used last on this device, or else the first one; '' without somas. */
  preferredSoma(): string {
    const last = storage(LAST_SOMA_KEY)
    return (this.somas.find((s) => s.id === last) ?? this.somas[0])?.id ?? ''
  }

  /** Opens a soma: its tree and live updates. Called when the URL names another soma. */
  useSoma(id: string): void {
    if (id === this.somaId && currentSoma() === id) return
    const prev = this.note
    if (prev) void prev.flush().finally(() => prev.close())
    this.note = null
    live.stop()
    this.somaId = id
    setSoma(id)
    this.entries = []
    this.treeLoaded = false
    this.expanded.clear()
    if (!id) return
    for (const p of loadExpanded(id)) this.expanded.add(p)
    storage(LAST_SOMA_KEY, id)
    void this.refresh()
    live.start({
      resync: () => {
        void this.refresh()
        void this.note?.revalidate()
      },
      change: (c) => this.applyChange(c),
      access: () => void this.refreshSomas(),
      collab: (name, data) => this.collabEvent(name, data),
      refused: () => void this.checkSession(),
    })
  }

  async createSoma(rawName: string): Promise<boolean> {
    const name = rawName.trim()
    if (!name) return false
    try {
      const created = await api.createSoma(name)
      this.somas = await api.listSomas()
      router.goSoma(created.id)
      this.toast(strings.somas.created(created.name), 'info')
      return true
    } catch (err) {
      this.toast(errorMessage(err))
      return false
    }
  }

  async renameSoma(id: string, rawName: string): Promise<boolean> {
    const name = rawName.trim()
    if (!name) return false
    try {
      await api.renameSoma(id, name)
      this.somas = this.somas.map((s) => (s.id === id ? { ...s, name } : s))
      return true
    } catch (err) {
      this.toast(errorMessage(err))
      return false
    }
  }

  /**
   * Re-reads the soma list after changes made elsewhere (sharing, admin area):
   * roles take effect at once, and a soma that is no longer accessible is left.
   */
  async refreshSomas(): Promise<void> {
    const before = this.soma
    try {
      this.somas = await api.listSomas()
    } catch {
      return
    }
    if (!this.somaId || this.somas.some((s) => s.id === this.somaId)) return
    if (before) this.toast(strings.somas.accessEnded(before.name), 'info')
    if (router.view === 'admin' || router.view === 'tutorials') this.useSoma(this.preferredSoma())
    else router.goSoma(this.preferredSoma(), true)
  }

  /** Leaves a soma that is shared with the account. */
  async leaveSoma(id: string): Promise<void> {
    const soma = this.somas.find((s) => s.id === id)
    if (!soma || soma.role === 'owner') return
    const ok = await this.ask({
      title: strings.share.leaveTitle,
      message: strings.share.leaveMessage(soma.name, soma.owner.displayName || soma.owner.username),
      confirmLabel: strings.share.leave,
      danger: true,
    })
    if (!ok) return
    try {
      await api.leaveSoma(id)
    } catch (err) {
      this.toast(errorMessage(err))
      return
    }
    this.shareOpen = false
    this.somas = this.somas.filter((s) => s.id !== id)
    this.toast(strings.share.left(soma.name), 'info')
    if (id === this.somaId) router.goSoma(this.preferredSoma(), true)
  }

  /** For changing actions: false (and a hint) when the role does not allow it. */
  may(what: Capability): boolean {
    if (this.can[what]) return true
    if (this.soma) this.toast(strings.share.notAllowed[what], 'info')
    return false
  }

  async deleteSoma(id: string): Promise<void> {
    const soma = this.somas.find((s) => s.id === id)
    if (!soma) return
    const ok = await this.ask({
      title: strings.somas.deleteTitle,
      message: strings.somas.deleteMessage(soma.name),
      confirmLabel: strings.somas.delete,
      danger: true,
    })
    if (!ok) return
    try {
      await api.deleteSoma(id)
    } catch (err) {
      this.toast(errorMessage(err))
      return
    }
    this.toast(strings.somas.deleted(soma.name), 'info')
    this.somas = await api.listSomas().catch(() => this.somas.filter((s) => s.id !== id))
    const next = this.somas[0]
    if (id === this.somaId) router.goSoma(next?.id ?? '', true)
  }

  private sessionEnded(): void {
    if (this.auth !== 'authenticated') return
    live.stop()
    this.closeNote()
    this.somaId = ''
    setSoma('')
    this.auth = 'anonymous'
    this.toast(strings.auth.sessionEnded, 'info')
  }

  /** The event stream was refused: find out whether we are still signed in. */
  private async checkSession(): Promise<void> {
    try {
      await api.me()
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) this.sessionEnded()
      return
    }
    // Still signed in: maybe the soma is no longer shared with us.
    await this.refreshSomas()
  }

  /** Applies a change reported by the server (made here or anywhere else). */
  private applyChange(c: Change): void {
    if (c.tree) {
      clearTimeout(this.refreshTimer)
      this.refreshTimer = setTimeout(() => void this.refresh(), 120)
    }
    const n = this.note
    if (!n) return
    // An open note follows its file through the server (editing together);
    // only a missing note looks again.
    if (c.notes.some((x) => x.path === n.path)) void n.changedOnDisk()
  }

  // --- Tree -------------------------------------------------------------

  async refresh(): Promise<void> {
    try {
      const t = await api.getTree()
      this.entries = t.entries
      this.orders = t.orders
      this.treeLoaded = true
      // Entries gone meanwhile leave the selection.
      if (this.selected.size) {
        const have = new Set(t.entries.map((e) => e.path))
        for (const p of [...this.selected]) if (!have.has(p)) this.selected.delete(p)
      }
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 401)) this.toast(errorMessage(err))
    }
  }

  /** Shows the inline name input for a new note, folder or drawing in parent. */
  startCreate(kind: NewEntryKind, parent: string): void {
    if (!this.may('create')) return
    if (parent) this.expanded.add(parent)
    this.edit = { kind, parent }
  }

  async createEntry(kind: 'note' | 'folder' | 'drawing', parent: string, rawName: string): Promise<boolean> {
    if (!this.may('create')) return false
    const problem = nameProblem(rawName)
    if (problem) {
      this.toast(problem)
      return false
    }
    let name = rawName.normalize('NFC')
    if (kind === 'note') name = withNoteExtension(name)
    if (kind === 'drawing') name = withDrawingExtension(name)
    let path = join(parent, name)
    try {
      if (kind === 'note') await api.createNote(path)
      else if (kind === 'drawing') path = (await drawingsApi.createDrawing(path, false)).path
      else await api.createFolder(path)
    } catch (err) {
      this.toast(errorMessage(err))
      return false
    }
    if (parent) this.expanded.add(parent)
    await this.refresh()
    if (kind === 'note' || kind === 'drawing') {
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
    // Keep a note a note: "Plan" for "Idea.md" becomes "Plan.md"; an
    // attachment keeps its type: "north bed" for "north.png" becomes "north bed.png".
    if (entry?.type === 'file' && isNote(path) && !isNote(name)) name += extname(path)
    else if (entry?.type === 'file' && isDrawingPath(path) && !extname(name)) name = withDrawingExtension(name)
    else if (entry?.type === 'file' && !isNote(path) && !extname(name)) name += extname(path)
    const to = join(dirname(path), name)
    if (to === path) return true
    return this.moveTo(path, to)
  }

  async moveInto(path: string, targetDir: string): Promise<boolean> {
    if (isInside(targetDir, path)) {
      if (targetDir !== path) this.toast(strings.tree.moveIntoSelf)
      return false
    }
    const to = join(targetDir, basename(path))
    if (to === path) return false
    if (!(await this.moveTo(path, to))) return false
    if (targetDir) this.expanded.add(targetDir)
    return true
  }

  /** Moves entries into a folder, one after another (a folder takes its own entries along). */
  async moveMany(paths: Iterable<string>, targetDir: string): Promise<number> {
    if (!this.may('organize')) return 0
    const list = outermost(paths).filter((p) => dirname(p) !== targetDir)
    if (list.length > 2) this.toast(strings.tree.moving(list.length), 'info')
    let moved = 0
    for (const p of list) if (await this.moveInto(p, targetDir)) moved++
    this.clearSelection()
    if (list.length > 1 && moved) this.toast(strings.tree.moved(moved), 'info')
    return moved
  }

  /** Drops entries into a folder sorted by hand, in front of `before` (moving them there first). */
  async dropInOrder(paths: readonly string[], folder: string, before: string | null): Promise<void> {
    if (!this.may('organize')) return
    const names: string[] = []
    for (const p of outermost(paths)) {
      if (dirname(p) === folder || (await this.moveInto(p, folder))) names.push(basename(p))
    }
    this.clearSelection()
    if (names.length) await this.placeInOrder(folder, names, before)
  }

  // --- Links ---------------------------------------------------------------

  /** The file whose link dialog is open. */
  linkDialog = $state<string | null>(null)

  /** Opens the entry an internal link points to (it may have moved). */
  async followLink(id: string): Promise<void> {
    try {
      const l = await linksApi.openLink(id)
      for (const dir of ancestors(l.path)) this.expanded.add(dir)
      router.go(l.path, true)
    } catch (err) {
      this.toast(err instanceof ApiError && err.code === 'not_found' ? strings.links.linkGone : errorMessage(err))
      router.go(null, true)
    }
  }

  // --- Sorting and selection ---------------------------------------------

  /** How a folder is sorted ("" is the top level). */
  orderOf(folder: string): FolderOrder {
    return this.orders[folder] ?? { mode: 'name' }
  }

  /** The entries of a folder as the tree shows them. */
  childrenOf(folder: string): TreeNode[] {
    if (folder === '') return this.tree
    let nodes = this.tree
    for (;;) {
      const next = nodes.find((n) => n.type === 'dir' && isInside(folder, n.path))
      if (!next) return []
      if (next.path === folder) return next.children
      nodes = next.children
    }
  }

  /** Sorts a folder for everyone; sorting by hand starts from the order shown now. */
  async setSort(folder: string, mode: SortMode): Promise<void> {
    if (!this.may('organize')) return
    await this.saveOrder(folder, mode === 'manual' ? { mode, names: this.childrenOf(folder).map((n) => n.name) } : { mode })
  }

  /** Puts entries of a folder in front of `before` (at the end when it is null), by hand. */
  async placeInOrder(folder: string, names: readonly string[], before: string | null): Promise<void> {
    if (!this.may('organize')) return
    const current = this.childrenOf(folder).map((n) => n.name)
    await this.saveOrder(folder, { mode: 'manual', names: reorder(current, names, before) })
  }

  private async saveOrder(folder: string, order: FolderOrder): Promise<void> {
    const before = this.orders
    // Shown at once; the change event brings the stored order to everyone.
    const next = { ...before, [folder]: order }
    if (order.mode === 'name') delete next[folder]
    this.orders = next
    try {
      await api.setOrder(folder, order)
    } catch (err) {
      this.orders = before
      this.toast(errorMessage(err))
    }
  }

  /** ⌘/Ctrl-click: adds or removes one entry (the open note joins the first time). */
  toggleSelected(path: string): void {
    const open = router.notePath
    if (this.selected.size === 0 && open && open !== path && this.entries.some((e) => e.path === open)) this.selected.add(open)
    if (this.selected.has(path)) this.selected.delete(path)
    else this.selected.add(path)
    this.selectAnchor = path
  }

  /** Shift-click: every row shown between the last picked entry (or the open note) and this one. */
  selectRange(path: string): void {
    const rows = visiblePaths(this.tree, this.expanded)
    const from = rows.indexOf(this.selectAnchor ?? router.notePath ?? '')
    const to = rows.indexOf(path)
    if (from < 0 || to < 0) {
      this.toggleSelected(path)
      return
    }
    const [a, b] = from < to ? [from, to] : [to, from]
    for (const p of rows.slice(a, b + 1)) this.selected.add(p)
  }

  clearSelection(): void {
    this.selected.clear()
    this.selectAnchor = null
  }

  private async moveTo(from: string, to: string): Promise<boolean> {
    if (!this.may('organize')) return false
    // Save first: the open note may be moved, or contain synapses to rewrite.
    await this.note?.flush()
    let res: api.MoveResult
    try {
      res = await api.move(from, to, settings.updateSynapses)
    } catch (err) {
      this.toast(errorMessage(err))
      await this.refresh()
      return false
    }
    const moved = res.path
    if (res.updatedSynapses || res.updatedEmbeds)
      this.toast(strings.toasts.synapsesUpdated(res.updatedSynapses ?? 0, res.updatedEmbeds ?? 0, res.updatedNotes ?? 0), 'info')
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
    if (!this.may('organize')) return
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

  /** Moves entries to the trash after one question (a folder takes its own entries along). */
  async removeMany(paths: Iterable<string>): Promise<void> {
    if (!this.may('organize')) return
    const list = outermost(paths)
    if (list.length <= 1) {
      if (list[0]) await this.remove(list[0])
      this.clearSelection()
      return
    }
    const ok = await this.ask({
      title: strings.tree.deleteManyTitle(list.length),
      message: strings.tree.deleteManyMessage(list.length),
      confirmLabel: strings.dialogs.moveToTrash,
      danger: true,
    })
    if (!ok) return
    let done = 0
    for (const p of list) {
      try {
        await api.trash(p)
        done++
      } catch (err) {
        this.toast(`${displayName(p)}: ${errorMessage(err)}`)
        continue
      }
      if (this.note && isInside(this.note.path, p)) {
        this.closeNote()
        router.go(null, true)
      }
      for (const e of [...this.expanded]) if (isInside(e, p)) this.expanded.delete(e)
    }
    this.clearSelection()
    await this.refresh()
    if (done) this.toast(strings.tree.movedManyToTrash(done), 'info')
  }

  // --- Navigation -------------------------------------------------------

  /** User-initiated navigation. */
  async navigate(path: string | null): Promise<void> {
    router.go(path)
  }

  /** The id of this tab's event stream, for joining notes (NoteHost). */
  stream(): Promise<string> {
    return live.waitStream()
  }

  /** Routes the events of editing together to the open note. */
  private collabEvent(name: CollabEventName, data: unknown): void {
    const n = this.note
    if (!n || data === null || typeof data !== 'object') return
    switch (name) {
      case 'collab':
        n.receive(data as CollabEvent)
        break
      case 'presence':
        n.presence(data as PresenceEvent)
        break
      case 'saved':
        n.saved(data as SavedEvent)
        break
      case 'notice': {
        const to = n.notice(data as NoticeEvent)
        if (to && router.notePath !== to && this.note === n) router.go(to, true)
        break
      }
    }
  }

  // --- Synapses ----------------------------------------------------------

  /** The note a synapse in fromPath points to, or null if it does not exist yet. */
  resolveSynapse(target: string, markdown: boolean, fromPath: string): string | null {
    return resolveSynapse({ target, markdown }, fromPath, this.notePaths)
  }

  /** The file an attachment reference in fromPath points to, or null. */
  resolveFile(target: string, markdown: boolean, fromPath: string): string | null {
    return resolveFile({ target, markdown }, fromPath, this.filePaths)
  }

  /**
   * Uploads files into a folder (taken names get a " (n)" suffix) and
   * returns the stored paths; failures are reported and skipped.
   */
  async upload(files: readonly File[], folder: string, rename?: (f: File) => string): Promise<string[]> {
    if (!this.may('create')) return []
    const stored: string[] = []
    for (const f of files) {
      const name = (rename?.(f) ?? f.name).normalize('NFC')
      const problem = nameProblem(name)
      if (problem) {
        this.toast(`${name}: ${problem}`)
        continue
      }
      try {
        stored.push((await api.uploadFile(join(folder, name), f)).path)
      } catch (err) {
        this.toast(strings.files.uploadFailed(name, errorMessage(err)))
      }
    }
    if (stored.length) {
      if (folder) this.expanded.add(folder)
      await this.refresh()
    }
    return stored
  }

  /** Uploads files dropped on the tree or picked in the file dialog, and says how it went. */
  async uploadInto(files: readonly File[], folder: string): Promise<void> {
    if (files.length > 1 || files.some((f) => f.size > 2 << 20)) this.toast(strings.files.uploading(files.length), 'info')
    const stored = await this.upload(files, folder)
    if (stored.length) this.toast(strings.files.uploaded(stored.length), 'info')
    // A single file opens right away.
    if (stored.length === 1 && files.length === 1) await this.navigate(stored[0]!)
  }

  /**
   * Uploads what was dropped from the computer, folders included: their
   * structure is created in folder (existing folders are used), the files go
   * into it. Problems are collected into one message.
   */
  async uploadDropped(entries: readonly FileSystemEntry[], folder: string): Promise<void> {
    if (!this.may('create')) return
    const dropped = await readDropped(entries, MAX_DROP_FILES)
    if (dropped.truncated) this.toast(strings.tree.tooManyFiles(MAX_DROP_FILES))
    if (dropped.files.length > 1) this.toast(strings.files.uploading(dropped.files.length), 'info')
    let skipped = 0
    let reason = ''
    const skip = (name: string, why: string) => {
      skipped++
      reason ||= `${name}: ${why}`
    }
    // Folders that could not be made: nothing goes into them.
    const failed: string[] = []
    const blocked = (dir: string) => failed.some((f) => isInside(dir, f))
    let made = 0
    for (const raw of dropped.folders) {
      const rel = raw.normalize('NFC')
      if (blocked(rel)) continue
      const problem = nameProblem(basename(rel))
      if (problem) {
        failed.push(rel)
        skip(rel, problem)
        continue
      }
      try {
        await api.createFolder(join(folder, rel))
        made++
      } catch (err) {
        if (err instanceof ApiError && err.code === 'exists') continue
        failed.push(rel)
        skip(rel, errorMessage(err))
      }
    }
    let stored = 0
    for (const { dir, file } of dropped.files) {
      const rel = dir.normalize('NFC')
      const name = file.name.normalize('NFC')
      if (rel && blocked(rel)) continue
      const problem = nameProblem(name)
      if (problem) {
        skip(name, problem)
        continue
      }
      try {
        await api.uploadFile(join(join(folder, rel), name), file)
        stored++
      } catch (err) {
        skip(name, errorMessage(err))
      }
    }
    if (folder) this.expanded.add(folder)
    for (const f of dropped.folders) if (!f.includes('/') && !blocked(f.normalize('NFC'))) this.expanded.add(join(folder, f.normalize('NFC')))
    await this.refresh()
    if (stored || made) this.toast(strings.tree.uploadedFolders(stored, made), 'info')
    if (skipped) this.toast(strings.tree.skippedFiles(skipped, reason))
  }

  /** How a stored file is best written in a note: its name, or its path if the name is ambiguous. */
  embedTarget(path: string, fromPath: string): string {
    const name = basename(path)
    return this.resolveFile(name, false, fromPath) === path ? name : path
  }

  /** Follows a synapse; an unresolved one creates its note first. */
  async openSynapse(target: string, markdown: boolean, fromPath: string): Promise<void> {
    const resolved = this.resolveSynapse(target, markdown, fromPath)
    if (resolved) {
      await this.navigate(resolved)
      return
    }
    if (!this.may('create')) return
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

  /** The note a drawing was opened from: "Done" goes back there. */
  drawingReturn: string | null = null

  /**
   * Asks for a name (a dated one filled in), creates the drawing next to the
   * open note, embeds it at the cursor and opens it in the editor.
   */
  async insertDrawing(): Promise<void> {
    const n = this.note
    if (!n || n.readOnly || !this.may('create')) return
    const now = new Date()
    const p = (v: number) => String(v).padStart(2, '0')
    const stamp = `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())} ${p(now.getHours())}${p(now.getMinutes())}`
    const raw = await this.prompt({
      title: strings.drawing.nameTitle,
      text: strings.drawing.nameText,
      label: strings.tree.drawingName,
      value: strings.drawing.defaultName(stamp),
      confirmLabel: strings.drawing.create,
    })
    if (raw === null || this.note !== n) return
    const name = withDrawingExtension(raw.trim().normalize('NFC'))
    const problem = nameProblem(name)
    if (problem) {
      this.toast(problem)
      return
    }
    let path: string
    try {
      path = (await drawingsApi.createDrawing(join(dirname(n.path), name))).path
    } catch (err) {
      this.toast(errorMessage(err))
      return
    }
    await this.refresh()
    if (this.note !== n || !n.insertBlock(`![[${this.embedTarget(path, n.path)}]]`)) return
    await this.navigate(path)
  }

  /** Leaves the drawing editor. */
  closeDrawing(path: string): void {
    const back = this.drawingReturn
    this.drawingReturn = null
    if (this.note?.path === path) void this.navigate(back && back !== path ? back : null)
  }

  /** Follows the URL: opens the note it names (or none). */
  openPath(path: string | null): void {
    if (this.note?.path === path) return
    const prev = this.note
    if (path && isDrawingPath(path)) {
      if (prev && isNote(prev.path)) this.drawingReturn = prev.path
    } else {
      this.drawingReturn = null
    }
    if (prev) void prev.flush().finally(() => prev.close())
    if (path === null) {
      this.note = null
      return
    }
    this.lastNotePath = path
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

  /** Asks for a line of text (e.g. a name); null if cancelled. */
  prompt(req: Omit<PromptRequest, 'resolve'>): Promise<string | null> {
    this.promptRequest?.resolve(null)
    return new Promise((resolve) => {
      this.promptRequest = { ...req, resolve }
    })
  }

  answerPrompt(value: string | null): void {
    const req = this.promptRequest
    this.promptRequest = null
    req?.resolve(value)
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
