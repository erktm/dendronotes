// Builds the CodeMirror editor for a note.

import { autocompletion, closeBrackets, closeBracketsKeymap, completionKeymap, type CompletionContext, type CompletionResult } from '@codemirror/autocomplete'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import { indentOnInput, syntaxHighlighting } from '@codemirror/language'
import { highlightSelectionMatches, search, searchKeymap } from '@codemirror/search'
import { Annotation, Compartment, EditorState, type Extension } from '@codemirror/state'
import { drawSelection, dropCursor, EditorView, highlightSpecialChars, keymap, placeholder } from '@codemirror/view'
import { tagHighlighter, tags } from '@lezer/highlight'
import { dirname, displayName } from '../paths'
import { livePreview, refreshDecorations, synapseHost, type SynapseHost } from './livePreview'
import { SynapseSyntax } from './synapseSyntax'

export interface EditorOptions {
  doc: string
  livePreview: boolean
  readOnly: boolean
  label: string
  placeholder: string
  host: SynapseHost
  /** Note paths offered when typing [[ . */
  notes: () => readonly string[]
  onChange: (doc: string) => void
  onCursor: (line: number, col: number) => void
  onVimMode: (mode: string | null) => void
}

export interface EditorHandle {
  doc(): string
  replaceDoc(text: string): void
  setLivePreview(on: boolean): void
  setReadOnly(on: boolean): void
  setVim(on: boolean): Promise<void>
  /** Re-resolves synapses (after notes were added, renamed or removed). */
  refresh(): void
  focus(): void
  /** Moves the cursor to a 0-based line and scrolls it to the top. */
  scrollToLine(line: number): void
  destroy(): void
}

/** Marks document replacements that come from outside (reload from disk). */
const external = Annotation.define<boolean>()

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
    autocompletion({ override: [synapseCompletion(o.notes)], icons: false }),
    search({ top: true }),
    highlightSelectionMatches(),
    keymap.of([...closeBracketsKeymap, ...defaultKeymap, ...searchKeymap, ...historyKeymap, ...completionKeymap, indentWithTab]),
    synapseHost.of(o.host),
    previewC.of(o.livePreview ? livePreview : []),
    readOnlyC.of(EditorState.readOnly.of(o.readOnly)),
    EditorView.contentAttributes.of({ 'aria-label': o.label, spellcheck: 'true' }),
    placeholder(o.placeholder),
    EditorView.updateListener.of((u) => {
      if (u.docChanged && !u.transactions.some((tr) => tr.annotation(external))) o.onChange(u.state.doc.toString())
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
    doc: () => view.state.doc.toString(),
    replaceDoc(text) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text }, annotations: external.of(true) })
    },
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
    scrollToLine(line) {
      const n = Math.min(Math.max(line + 1, 1), view.state.doc.lines)
      const l = view.state.doc.line(n)
      view.dispatch({ selection: { anchor: l.from }, effects: EditorView.scrollIntoView(l.from, { y: 'start', yMargin: 24 }) })
      view.focus()
    },
    destroy() {
      view.destroy()
    },
  }
}
