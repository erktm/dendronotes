// An open drawing in the editor: its scene, undo and redo, and saving – about
// a second after the last change, guarded by the file's ETag. Nothing is
// overwritten silently: when someone else saved meanwhile, the editor asks.

import { ApiError } from '../api/client'
import * as api from '../api/drawings'
import { measureText } from './dom'
import { type Scene, emptyScene } from './scene'
import { parseDrawing, toSVG } from './svg'

export type DrawingStatus = 'loading' | 'ready' | 'missing' | 'not-a-drawing' | 'error'
/** saved: nothing to do; pending: a save is coming; conflict: changed elsewhere meanwhile. */
export type SaveState = 'saved' | 'pending' | 'saving' | 'failed' | 'conflict'

const SAVE_DELAY = 1000
const RETRY_DELAY = 5000
const MAX_UNDO = 200

export class DrawingDoc {
  status = $state<DrawingStatus>('loading')
  scene = $state.raw<Scene>(emptyScene())
  save = $state<SaveState>('saved')
  /** Why the last save failed (an API error code). */
  failure = $state('')
  private undoStack = $state.raw<Scene[]>([])
  private redoStack = $state.raw<Scene[]>([])
  canUndo = $derived(this.undoStack.length > 0)
  canRedo = $derived(this.redoStack.length > 0)

  private etag = ''
  private timer: ReturnType<typeof setTimeout> | null = null
  private saving: Promise<void> | null = null
  private closed = false

  constructor(readonly path: string) {}

  /** Changes exist that are not in the file yet. */
  get dirty(): boolean {
    return this.save !== 'saved'
  }

  async load(): Promise<void> {
    this.status = 'loading'
    try {
      const f = await api.readDrawing(this.path)
      const scene = parseDrawing(f.svg ?? '')
      if (!scene) {
        this.status = 'not-a-drawing'
        return
      }
      this.etag = f.etag
      this.scene = scene
      this.undoStack = []
      this.redoStack = []
      this.save = 'saved'
      this.status = 'ready'
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) this.status = 'missing'
      else if (err instanceof ApiError && err.code === 'not_a_drawing') this.status = 'not-a-drawing'
      else this.status = 'error'
    }
  }

  /** Shows a scene while a gesture runs: no undo step, nothing saved yet. */
  preview(scene: Scene): void {
    this.scene = scene
  }

  /** Ends a change: one undo step back to before, and a save soon. */
  commit(scene: Scene, before: Scene = this.scene): void {
    this.scene = scene
    if (scene === before) return
    this.undoStack = [...this.undoStack.slice(-(MAX_UNDO - 1)), before]
    this.redoStack = []
    this.schedule()
  }

  undo(): void {
    const prev = this.undoStack.at(-1)
    if (!prev) return
    this.undoStack = this.undoStack.slice(0, -1)
    this.redoStack = [...this.redoStack, this.scene]
    this.scene = prev
    this.schedule()
  }

  redo(): void {
    const next = this.redoStack.at(-1)
    if (!next) return
    this.redoStack = this.redoStack.slice(0, -1)
    this.undoStack = [...this.undoStack, this.scene]
    this.scene = next
    this.schedule()
  }

  private schedule(delay = SAVE_DELAY): void {
    if (this.save === 'conflict') return
    if (this.save !== 'saving') this.save = 'pending'
    if (this.timer) clearTimeout(this.timer)
    this.timer = setTimeout(() => void this.flush(), delay)
  }

  /** Saves now if anything is waiting. */
  async flush(): Promise<void> {
    if (this.timer) {
      clearTimeout(this.timer)
      this.timer = null
    }
    if (this.saving) await this.saving
    if (this.save !== 'pending' && this.save !== 'failed') return
    this.saving = this.write().finally(() => (this.saving = null))
    await this.saving
  }

  private async write(): Promise<void> {
    const scene = this.scene
    this.save = 'saving'
    try {
      const r = await api.saveDrawing(this.path, toSVG(scene, measureText()), this.etag)
      this.etag = r.etag
      this.failure = ''
      // Changes made while saving go in the next save.
      this.save = this.scene === scene ? 'saved' : 'pending'
      if (this.save === 'pending') this.schedule(0)
    } catch (err) {
      if (err instanceof ApiError && err.status === 412) {
        this.save = 'conflict'
        return
      }
      this.save = 'failed'
      this.failure = err instanceof ApiError ? err.code : 'network'
      // Lost connections come back; refusals (too large) wait for the next change.
      if (!this.closed && this.failure === 'network') this.timer = setTimeout(() => void this.flush(), RETRY_DELAY)
    }
  }

  /** After a conflict: take the other version and drop the own changes (they stay undoable). */
  async takeTheirs(): Promise<void> {
    const mine = this.scene
    const undo = this.undoStack
    await this.load()
    if (this.status === 'ready') this.undoStack = [...undo, mine]
  }

  /** After a conflict: write the own version over the other one. */
  async keepMine(): Promise<void> {
    try {
      const f = await api.readDrawing(this.path)
      this.etag = f.etag
    } catch {
      // Saving tells what is wrong.
    }
    this.save = 'pending'
    await this.flush()
  }

  /** The file changed (the tree says so): without own changes, show the new version. */
  async changedElsewhere(): Promise<void> {
    if (this.save !== 'saved' || this.status !== 'ready') return
    try {
      const f = await api.readDrawing(this.path)
      if (f.etag === this.etag || this.save !== 'saved') return
      const scene = parseDrawing(f.svg ?? '')
      if (!scene) return
      this.etag = f.etag
      this.scene = scene
      this.undoStack = []
      this.redoStack = []
    } catch {
      // The next save finds out.
    }
  }

  close(): void {
    this.closed = true
    if (this.dirty && this.save !== 'conflict') void this.flush()
  }
}
