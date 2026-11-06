import { ApiError } from '../api/client'
import { readNote, saveNote } from '../api/soma'
import { errorMessage } from '../errors'
import { displayName, isNote } from '../paths'
import { strings } from '../strings'

export type NoteStatus = 'loading' | 'ready' | 'missing' | 'unsupported' | 'too-large' | 'error'

export interface NoteHost {
  toast(text: string, kind?: 'error' | 'info'): void
}

/** How many of our own recent versions are remembered (see changedOnDisk). */
const KNOWN_VERSIONS = 16

const AUTOSAVE_DELAY = 800

/**
 * One open note: its content, the version it is based on (ETag) and the
 * save state. Edits are saved automatically shortly after typing stops.
 * Every save sends If-Match, so a change made elsewhere in the meantime is
 * never overwritten silently – it becomes a conflict the user resolves.
 */
export class NoteSession {
  path = $state('')
  status = $state<NoteStatus>('loading')
  content = $state('')
  saved = $state('')
  readOnly = $state(false)
  /** ETag of the version on disk when a save hit a conflict. */
  conflict = $state<string | null>(null)
  /** Content of the version on disk during a conflict (for the comparison). */
  disk = $state<string | null>(null)
  saving = $state(false)
  failed = $state(false)
  /** Put the cursor into the editor once loaded (e.g. for a new note). */
  autofocus = $state(false)
  dirty = $derived(this.content !== this.saved)

  private host: NoteHost
  private etag = ''
  private timer: ReturnType<typeof setTimeout> | undefined
  private inflight: Promise<void> | null = null
  private closed = false
  /** Versions this session loaded or saved itself: events about them are no news. */
  private known: string[] = []

  constructor(path: string, host: NoteHost) {
    this.path = path
    this.host = host
  }

  async load(): Promise<void> {
    if (!isNote(this.path)) {
      this.status = 'unsupported'
      return
    }
    this.status = 'loading'
    try {
      const n = await readNote(this.path)
      if (this.closed) return
      this.setVersion(n.etag)
      this.content = n.content
      this.saved = n.content
      this.readOnly = !!n.readOnly
      this.conflict = null
      this.disk = null
      this.failed = false
      this.status = 'ready'
    } catch (err) {
      if (this.closed) return
      if (err instanceof ApiError && err.status === 404) this.status = 'missing'
      else if (err instanceof ApiError && err.code === 'not_a_note') this.status = 'unsupported'
      else if (err instanceof ApiError && err.code === 'too_large') this.status = 'too-large'
      else this.status = 'error'
    }
  }

  /** Call after every edit; saves once typing pauses. */
  scheduleSave(): void {
    clearTimeout(this.timer)
    if (this.readOnly || this.conflict !== null || this.closed) return
    this.timer = setTimeout(() => void this.save(), AUTOSAVE_DELAY)
  }

  /** Saves now. With overwrite, a conflicting version on disk is replaced. */
  async save(overwrite = false): Promise<void> {
    clearTimeout(this.timer)
    if (this.inflight) await this.inflight
    if (this.status !== 'ready' || this.readOnly) return
    if (!this.dirty && !overwrite) return
    if (this.conflict !== null && !overwrite) return

    const text = this.content
    const basedOn = overwrite && this.conflict !== null ? this.conflict : this.etag
    this.inflight = (async () => {
      this.saving = true
      try {
        const meta = await saveNote(this.path, text, basedOn)
        this.setVersion(meta.etag)
        this.saved = text
        this.conflict = null
        this.disk = null
        this.failed = false
      } catch (err) {
        if (err instanceof ApiError && err.status === 412 && typeof err.body.etag === 'string') {
          this.conflict = err.body.etag
        } else {
          this.failed = true
          if (!this.closed) this.host.toast(strings.toasts.saveFailed(errorMessage(err)))
        }
      } finally {
        this.saving = false
        this.inflight = null
      }
    })()
    await this.inflight
    // Typing continued while saving: save the rest, too.
    if (this.dirty && this.conflict === null && !this.failed) this.scheduleSave()
  }

  /** Saves pending changes, e.g. before switching notes or moving files. */
  async flush(): Promise<void> {
    clearTimeout(this.timer)
    if (this.inflight) await this.inflight
    if (this.dirty && this.conflict === null) await this.save()
  }

  /**
   * Loads the version on disk for the comparison. Its ETag becomes the
   * conflict version, so "Keep mine" replaces exactly what was shown.
   */
  async loadDisk(): Promise<void> {
    try {
      const n = await readNote(this.path)
      if (this.closed || this.conflict === null) return
      this.disk = n.content
      this.conflict = n.etag
    } catch (err) {
      if (!this.closed) this.host.toast(errorMessage(err))
    }
  }

  /** Throws away local changes and loads the version on disk. */
  async reloadFromDisk(): Promise<void> {
    clearTimeout(this.timer)
    await this.load()
  }

  close(): void {
    this.closed = true
    clearTimeout(this.timer)
  }

  private setVersion(etag: string): void {
    this.etag = etag
    this.known = [etag, ...this.known.filter((e) => e !== etag)].slice(0, KNOWN_VERSIONS)
  }

  /**
   * The note changed on disk (reported by the event stream). Without unsaved
   * edits it reloads quietly; with them, the conflict view opens. Our own
   * saves come back as events too – sometimes before the save's answer –
   * and are recognized by their version.
   */
  async changedOnDisk(etag: string): Promise<void> {
    if (this.status === 'missing' && !this.closed) {
      await this.load() // the note is back (restored, or moved back)
      return
    }
    if (this.inflight) await this.inflight
    if (this.closed || this.status !== 'ready' || this.known.includes(etag)) return
    if (this.conflict !== null) {
      if (etag !== this.conflict) {
        this.conflict = etag
        this.disk = null // the comparison loads the newer version
      }
      return
    }
    if (this.dirty) {
      this.conflict = etag
      return
    }
    try {
      const n = await readNote(this.path)
      if (this.closed || this.known.includes(n.etag)) return
      if (this.dirty || this.inflight) {
        // Typing started while loading: let the user decide.
        this.conflict = n.etag
        this.disk = n.content
        return
      }
      this.setVersion(n.etag)
      this.content = n.content
      this.saved = n.content
      this.readOnly = !!n.readOnly
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) this.removedOnDisk()
    }
  }

  /** The note was deleted or moved away elsewhere. */
  removedOnDisk(): void {
    if (this.closed || this.status !== 'ready') return
    if (this.dirty) {
      this.host.toast(strings.toasts.deletedElsewhere(displayName(this.path)), 'info')
      return
    }
    clearTimeout(this.timer)
    this.status = 'missing'
  }

  /** After reconnecting: catch up with changes that happened meanwhile. */
  async revalidate(): Promise<void> {
    if (this.closed) return
    if (this.status === 'missing' || this.status === 'error') {
      await this.load()
      return
    }
    if (this.status !== 'ready') return
    try {
      const n = await readNote(this.path)
      if (n.etag !== this.etag) await this.changedOnDisk(n.etag)
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) this.removedOnDisk()
    }
  }
}
