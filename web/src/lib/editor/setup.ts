// Builds the CodeMirror editor for a note.

import { autocompletion, closeBrackets, closeBracketsKeymap, completionKeymap, type CompletionContext, type CompletionResult } from '@codemirror/autocomplete'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import { indentOnInput, syntaxHighlighting } from '@codemirror/language'
import { highlightSelectionMatches, search, searchKeymap } from '@codemirror/search'
import { Compartment, EditorSelection, EditorState, type Extension } from '@codemirror/state'
import { drawSelection, dropCursor, EditorView, highlightSpecialChars, keymap, placeholder } from '@codemirror/view'
import { tagHighlighter, tags } from '@lezer/highlight'
import { outline, type Heading } from '../outline'
import { dirname, displayName } from '../paths'
import { livePreview, refreshDecorations, synapseHost, type SynapseHost } from './livePreview'
import { presence, setPeers, type PeerCursor } from './presence'
import { SynapseSyntax } from './synapseSyntax'

export interface EditorOptions {
  doc: string
  /** The collaborative editing plugin (@codemirror/collab) of the note. */
  collab: Extension
  livePreview: boolean
  readOnly: boolean
  label: string
  placeholder: string
  host: SynapseHost
  /** Note paths offered when typing [[ . */
  notes: () => readonly string[]
  /** Headings of another note, offered when typing [[Note# . */
  headings: (path: string) => Promise<readonly Heading[]>
  /** The document changed (typing, or updates from others). */
  onChange: (view: EditorView) => void
  /** The selection moved (also when the document changed). */
  onSelection: (view: EditorView) => void
  onCursor: (line: number, col: number) => void
  onVimMode: (mode: string | null) => void
  /** Files pasted or dropped into the editor at a position. */
  onFiles: (files: File[], pos: number) => void
}

export interface EditorHandle {
  readonly view: EditorView
  setLivePreview(on: boolean): void
  setReadOnly(on: boolean): void
  setVim(on: boolean): Promise<void>
  /** Re-resolves synapses (after notes were added, renamed or removed). */
  refresh(): void
  focus(): void
  /** Moves the cursor to a 0-based line and scrolls it to the top. */
  scrollToLine(line: number): void
  /** Inserts text at a position (as a user edit) and puts the cursor after it. */
  insertAt(pos: number, text: string): void
  /** Shows the cursors of the others. */
  setPeers(peers: PeerCursor[]): void
  /** Puts back cursor and scroll position (after the editor started over). */
  restore(sel: { anchor: number; head: number } | null, scrollTop: number): void
  /** Measures again (after being hidden). */
  remeasure(): void
  destroy(): void
}

// Token classes; colors and sizes live in styles/editor.css.
const highlighter = tagHighlighter([
  { tag: tags.heading, class: 'tok-heading' },
  { tag: tags.emphasis, class: 'tok-em' },
  { tag: tags.strong, class: 'tok-strong' },
  { tag: tags.strikethrough, class: 'tok-strike' },
  { tag: [tags.link, tags.url], class: 'tok-link' },
  { tag: tags.monospace, class: 'tok-code' },
  { tag: tags.quote, class: 'tok-quote' },
  { tag: [tags.processingInstruction, tags.meta, tags.contentSeparator, tags.labelName], class: 'tok-mark' },
])

/** [[Note#…: the headings of that note; [[#…: those of this note. */
function headingCompletion(o: EditorOptions) {
  return async (ctx: CompletionContext): Promise<CompletionResult | null> => {
    const before = ctx.matchBefore(/\[\[[^[\]\n|#]*#[^[\]\n|#]*$/)
    if (!before) return null
    const inner = before.text.slice(2)
    const hash = inner.indexOf('#')
    const target = inner.slice(0, hash).trim()
    let headings: readonly Heading[]
    if (!target) {
      headings = outline(ctx.state.doc.toString())
    } else {
      const path = o.host.resolve(target, false)
      if (!path) return null
      try {
        headings = await o.headings(path)
      } catch {
        return null
      }
    }
    if (ctx.aborted || headings.length === 0) return null
    return {
      from: before.from + 2 + hash + 1,
      validFor: /^[^[\]\n|#]*$/,
      options: headings.map((h, i) => ({
        label: h.text,
        detail: `H${h.level}`,
        boost: -i, // keep the note's order
        apply: (view: EditorView, _c: unknown, from: number, to: number) => {
          const closed = view.state.sliceDoc(to, to + 2) === ']]'
          view.dispatch({
            changes: { from, to, insert: closed ? h.text : `${h.text}]]` },
            selection: { anchor: from + h.text.length + 2 },
          })
        },
      })),
    }
  }
}

function synapseCompletion(notes: () => readonly string[]) {
  return (ctx: CompletionContext): CompletionResult | null => {
    const before = ctx.matchBefore(/\[\[[^[\]\n|#]*$/)
    if (!before) return null
    const list = notes()
    const counts = new Map<string, number>()
    for (const p of list) {
      const key = displayName(p).toLowerCase()
      counts.set(key, (counts.get(key) ?? 0) + 1)
    }
    return {
      from: before.from + 2,
      validFor: /^[^[\]\n|#]*$/,
      options: list.map((path) => {
        const name = displayName(path)
        // Names that occur more than once are written with their path.
        const insert = counts.get(name.toLowerCase()) === 1 ? name : path.replace(/\.md$/i, '')
        return {
          label: name,
          detail: dirname(path) || undefined,
          apply: (view: EditorView, _c: unknown, from: number, to: number) => {
            const closed = view.state.sliceDoc(to, to + 2) === ']]'
            view.dispatch({
              changes: { from, to, insert: closed ? insert : `${insert}]]` },
              selection: { anchor: from + insert.length + 2 },
            })
          },
        }
      }),
    }
  }
}

export function createEditor(parent: HTMLElement, o: EditorOptions): EditorHandle {
  const previewC = new Compartment()
  const readOnlyC = new Compartment()
  const vimC = new Compartment()
  const nonce = document.querySelector<HTMLMetaElement>('meta[name="csp-nonce"]')?.content ?? ''

  const extensions: Extension[] = [
    vimC.of([]), // Vim must come first so its keys win.
    EditorView.cspNonce.of(nonce),
    history(),
    drawSelection(),
    dropCursor(),
    indentOnInput(),
    highlightSpecialChars(),
    EditorView.lineWrapping,
    markdown({ base: markdownLanguage, extensions: [SynapseSyntax] }),
    syntaxHighlighting(highlighter),
    closeBrackets(),
    autocompletion({ override: [synapseCompletion(o.notes), headingCompletion(o)], icons: false }),
    search({ top: true }),
    highlightSelectionMatches(),
    keymap.of([...closeBracketsKeymap, ...defaultKeymap, ...searchKeymap, ...historyKeymap, ...completionKeymap, indentWithTab]),
    synapseHost.of(o.host),
    o.collab,
    presence,
    previewC.of(o.livePreview ? livePreview : []),
    readOnlyC.of(EditorState.readOnly.of(o.readOnly)),
    EditorView.contentAttributes.of({ 'aria-label': o.label, spellcheck: 'true' }),
    placeholder(o.placeholder),
    // Pasted or dropped files are uploaded and embedded where they land.
    EditorView.domEventHandlers({
      paste(event, view) {
        const files = [...(event.clipboardData?.files ?? [])]
        if (files.length === 0 || view.state.readOnly) return false
        event.preventDefault()
        o.onFiles(files, view.state.selection.main.head)
        return true
      },
      drop(event, view) {
        const files = [...(event.dataTransfer?.files ?? [])]
        if (files.length === 0 || view.state.readOnly) return false
        event.preventDefault()
        const pos = view.posAtCoords({ x: event.clientX, y: event.clientY }) ?? view.state.selection.main.head
        o.onFiles(files, pos)
        return true
      },
    }),
    EditorView.updateListener.of((u) => {
      if (u.docChanged) o.onChange(u.view)
      if (u.docChanged || u.selectionSet || u.focusChanged) o.onSelection(u.view)
      if (u.docChanged || u.selectionSet) {
        const head = u.state.selection.main.head
        const line = u.state.doc.lineAt(head)
        o.onCursor(line.number, head - line.from + 1)
      }
    }),
  ]

  const view = new EditorView({ parent, state: EditorState.create({ doc: o.doc, extensions }) })
  let vimOn = false

  return {
    view,
    setLivePreview(on) {
      view.dispatch({ effects: previewC.reconfigure(on ? livePreview : []) })
    },
    setReadOnly(on) {
      view.dispatch({ effects: readOnlyC.reconfigure(EditorState.readOnly.of(on)) })
    },
    async setVim(on) {
      if (on === vimOn) return
      vimOn = on
      if (!on) {
        view.dispatch({ effects: vimC.reconfigure([]) })
        o.onVimMode(null)
        return
      }
      const { vim, getCM } = await import('@replit/codemirror-vim')
      if (!vimOn) return
      view.dispatch({ effects: vimC.reconfigure(vim()) })
      o.onVimMode('normal')
      getCM(view)?.on('vim-mode-change', (e: { mode: string; subMode?: string }) =>
        o.onVimMode(e.subMode ? `${e.mode} ${e.subMode}` : e.mode),
      )
    },
    refresh() {
      view.dispatch({ effects: refreshDecorations.of(null) })
    },
    focus() {
      view.focus()
    },
    insertAt(pos, text) {
      const at = Math.min(Math.max(pos, 0), view.state.doc.length)
      view.dispatch({ changes: { from: at, insert: text }, selection: { anchor: at + text.length }, userEvent: 'input.paste' })
      view.focus()
    },
    scrollToLine(line) {
      const n = Math.min(Math.max(line + 1, 1), view.state.doc.lines)
      const l = view.state.doc.line(n)
      view.dispatch({ selection: { anchor: l.from }, effects: EditorView.scrollIntoView(l.from, { y: 'start', yMargin: 24 }) })
      view.focus()
    },
    setPeers(peers) {
      view.dispatch({ effects: setPeers.of(peers) })
    },
    restore(sel, scrollTop) {
      if (sel) {
        const n = view.state.doc.length
        view.dispatch({ selection: EditorSelection.single(Math.min(sel.anchor, n), Math.min(sel.head, n)) })
      }
      view.scrollDOM.scrollTop = scrollTop
    },
    remeasure() {
      view.requestMeasure()
    },
    destroy() {
      view.destroy()
    },
  }
}
