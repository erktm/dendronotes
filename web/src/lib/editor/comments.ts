// Comments in the editor: the text a thread is about is underlined (and
// follows edits), clicking it focuses the thread, and a small button next to
// a selection starts a new one. Classes only, no inline styles (CSP).

import { StateEffect, StateField, type EditorState, type Range } from '@codemirror/state'
import { Decoration, EditorView, showTooltip, type DecorationSet, type Tooltip } from '@codemirror/view'

export interface CommentRange {
  id: number
  from: number
  to: number
  active: boolean
  resolved: boolean
}

export interface CommentHost {
  /** The button next to a selection was pressed. */
  comment(from: number, to: number): void
  /** A commented passage was clicked. */
  focus(id: number): void
  label: string
}

/** Replaces the commented passages (positions in the current document). */
export const setCommentRanges = StateEffect.define<CommentRange[]>()

function build(ranges: readonly CommentRange[], length: number): DecorationSet {
  const out: Range<Decoration>[] = []
  for (const r of ranges) {
    const from = Math.min(Math.max(r.from, 0), length)
    const to = Math.min(Math.max(r.to, 0), length)
    if (from >= to || r.resolved) continue
    out.push(
      Decoration.mark({
        class: r.active ? 'cm-comment cm-comment-active' : 'cm-comment',
        attributes: { 'data-thread': String(r.id) },
      }).range(from, to),
    )
  }
  return Decoration.set(out, true)
}

const marks = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    deco = deco.map(tr.changes)
    for (const e of tr.effects) if (e.is(setCommentRanges)) deco = build(e.value, tr.state.doc.length)
    return deco
  },
  provide: (f) => EditorView.decorations.from(f),
})

/** Where each thread's passage is now (after the edits since it was set). */
export function commentPositions(state: EditorState): Map<number, { from: number; to: number }> {
  const out = new Map<number, { from: number; to: number }>()
  state.field(marks, false)?.between(0, state.doc.length, (from, to, d) => {
    const id = Number((d.spec.attributes as Record<string, string> | undefined)?.['data-thread'])
    if (id) out.set(id, { from, to })
  })
  return out
}

function selectionTooltip(host: CommentHost) {
  const make = (state: EditorState): Tooltip | null => {
    const sel = state.selection.main
    if (sel.empty || state.sliceDoc(sel.from, sel.to).trim() === '') return null
    return {
      pos: sel.to,
      above: false,
      strictSide: false,
      arrow: false,
      create: (view) => {
        const dom = document.createElement('div')
        dom.className = 'cm-comment-tip'
        const button = document.createElement('button')
        button.type = 'button'
        button.textContent = host.label
        // Keep the selection: act on mousedown, before the editor sees the click.
        button.addEventListener('mousedown', (e) => {
          e.preventDefault()
          const s = view.state.selection.main
          if (!s.empty) host.comment(s.from, s.to)
        })
        dom.append(button)
        return { dom }
      },
    }
  }
  return StateField.define<Tooltip | null>({
    create: make,
    update(tip, tr) {
      if (!tr.docChanged && !tr.selection) return tip
      return make(tr.state)
    },
    provide: (f) => showTooltip.from(f),
  })
}

/** The comment extension for an editor. */
export function commentExtension(host: CommentHost) {
  return [
    marks,
    selectionTooltip(host),
    EditorView.domEventHandlers({
      click(event) {
        const el = (event.target as HTMLElement | null)?.closest<HTMLElement>('.cm-comment')
        const id = Number(el?.dataset.thread)
        if (!id) return false
        host.focus(id)
        return false // the cursor still goes where clicked
      },
    }),
  ]
}
