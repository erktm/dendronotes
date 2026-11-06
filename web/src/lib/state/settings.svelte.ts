// Per-browser settings, stored in localStorage. Nothing is written into the
// soma, so the notes stay plain Markdown.

export type ThemeChoice = 'system' | 'light' | 'dark'
export type ViewMode = 'edit' | 'read'
export type NoteFont = 'sans' | 'serif'

const KEY = 'dendronotes.settings'

interface Stored {
  livePreview: boolean
  vim: boolean
  readableWidth: boolean
  noteFont: NoteFont
  theme: ThemeChoice
  showDetails: boolean
  view: ViewMode
}

const DEFAULTS: Stored = {
  livePreview: true,
  vim: false,
  readableWidth: true,
  noteFont: 'sans',
  theme: 'system',
  showDetails: true,
  view: 'edit',
}

function load(): Stored {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(KEY) ?? '{}')
    if (raw === null || typeof raw !== 'object') return { ...DEFAULTS }
    const r = raw as Partial<Record<keyof Stored, unknown>>
    const bool = (k: keyof Stored) => (typeof r[k] === 'boolean' ? (r[k] as boolean) : (DEFAULTS[k] as boolean))
    const pick = <T extends string>(k: keyof Stored, allowed: readonly T[]) =>
      allowed.includes(r[k] as T) ? (r[k] as T) : (DEFAULTS[k] as T)
    return {
      livePreview: bool('livePreview'),
      vim: bool('vim'),
      readableWidth: bool('readableWidth'),
      noteFont: pick('noteFont', ['sans', 'serif'] as const),
      theme: pick('theme', ['system', 'light', 'dark'] as const),
      showDetails: bool('showDetails'),
      view: pick('view', ['edit', 'read'] as const),
    }
  } catch {
    return { ...DEFAULTS }
  }
}

class Settings {
  livePreview: boolean
  vim: boolean
  readableWidth: boolean
  noteFont: NoteFont
  theme: ThemeChoice
  showDetails: boolean
  view: ViewMode

  constructor() {
    const s = load()
    this.livePreview = $state(s.livePreview)
    this.vim = $state(s.vim)
    this.readableWidth = $state(s.readableWidth)
    this.noteFont = $state(s.noteFont)
    this.theme = $state(s.theme)
    this.showDetails = $state(s.showDetails)
    this.view = $state(s.view)

    $effect.root(() => {
      $effect(() => {
        const snapshot: Stored = {
          livePreview: this.livePreview,
          vim: this.vim,
          readableWidth: this.readableWidth,
          noteFont: this.noteFont,
          theme: this.theme,
          showDetails: this.showDetails,
          view: this.view,
        }
        try {
          localStorage.setItem(KEY, JSON.stringify(snapshot))
        } catch {
          // Storage disabled: settings last for this page only.
        }
      })
      $effect(() => {
        const root = document.documentElement
        if (this.theme === 'system') delete root.dataset.theme
        else root.dataset.theme = this.theme
      })
    })
  }

  toggleView(): void {
    this.view = this.view === 'edit' ? 'read' : 'edit'
  }
}

export const settings = new Settings()
