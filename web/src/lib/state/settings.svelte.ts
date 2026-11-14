// Per-browser settings, stored in localStorage. Nothing is written into the
// soma, so the notes stay plain Markdown.

import { DEFAULT_PALETTE, DRAWN, PALETTES, THEME_COLORS, type Palette } from '../palettes'

export type ThemeChoice = 'system' | 'light' | 'dark'
export type ViewMode = 'edit' | 'read'
export type NoteFont = 'sans' | 'serif'
export type GraphMode = 'global' | 'local'

const KEY = 'dendronotes.settings'

interface Stored {
  livePreview: boolean
  vim: boolean
  readableWidth: boolean
  noteFont: NoteFont
  theme: ThemeChoice
  palette: Palette
  showDetails: boolean
  view: ViewMode
  updateSynapses: boolean
  graphMode: GraphMode
  graphDepth: number
  graphMissing: boolean
  graphOrphans: boolean
  graphDrawings: boolean
}

const DEFAULTS: Stored = {
  livePreview: true,
  vim: false,
  readableWidth: true,
  noteFont: 'sans',
  theme: 'system',
  palette: DEFAULT_PALETTE,
  showDetails: true,
  view: 'edit',
  updateSynapses: true,
  graphMode: 'global',
  graphDepth: 2,
  graphMissing: true,
  graphOrphans: true,
  graphDrawings: true,
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
      palette: pick('palette', PALETTES),
      showDetails: bool('showDetails'),
      view: pick('view', ['edit', 'read'] as const),
      updateSynapses: bool('updateSynapses'),
      graphMode: pick('graphMode', ['global', 'local'] as const),
      graphDepth: [1, 2, 3].includes(r.graphDepth as number) ? (r.graphDepth as number) : DEFAULTS.graphDepth,
      graphMissing: bool('graphMissing'),
      graphOrphans: bool('graphOrphans'),
      graphDrawings: bool('graphDrawings'),
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
  palette: Palette
  showDetails: boolean
  view: ViewMode
  updateSynapses: boolean
  graphMode: GraphMode
  graphDepth: number
  graphMissing: boolean
  graphOrphans: boolean
  graphDrawings: boolean

  constructor() {
    const s = load()
    this.livePreview = $state(s.livePreview)
    this.vim = $state(s.vim)
    this.readableWidth = $state(s.readableWidth)
    this.noteFont = $state(s.noteFont)
    this.theme = $state(s.theme)
    this.palette = $state(s.palette)
    this.showDetails = $state(s.showDetails)
    this.view = $state(s.view)
    this.updateSynapses = $state(s.updateSynapses)
    this.graphMode = $state(s.graphMode)
    this.graphDepth = $state(s.graphDepth)
    this.graphMissing = $state(s.graphMissing)
    this.graphOrphans = $state(s.graphOrphans)
    this.graphDrawings = $state(s.graphDrawings)

    $effect.root(() => {
      $effect(() => {
        const snapshot: Stored = {
          livePreview: this.livePreview,
          vim: this.vim,
          readableWidth: this.readableWidth,
          noteFont: this.noteFont,
          theme: this.theme,
          palette: this.palette,
          showDetails: this.showDetails,
          view: this.view,
          updateSynapses: this.updateSynapses,
          graphMode: this.graphMode,
          graphDepth: this.graphDepth,
          graphMissing: this.graphMissing,
          graphOrphans: this.graphOrphans,
          graphDrawings: this.graphDrawings,
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
      $effect(() => {
        const root = document.documentElement
        root.dataset.palette = this.palette
        root.toggleAttribute('data-drawn', DRAWN.has(this.palette))
        const colors = THEME_COLORS[this.palette]
        for (const meta of document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) {
          meta.content = meta.media.includes('dark') ? colors.dark : colors.light
        }
      })
    })
  }

  toggleView(): void {
    this.view = this.view === 'edit' ? 'read' : 'edit'
  }
}

export const settings = new Settings()
