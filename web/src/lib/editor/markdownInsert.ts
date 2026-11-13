// Common Markdown at the cursor (the editor's right-click menu): headings,
// emphasis, lists, tasks, quotes, code, tables, links and rules. Line marks
// replace the ones a line has and come off again when every line has them;
// marks around text wrap the selection, or leave the cursor between them.

import { EditorSelection, type EditorState, type TransactionSpec } from '@codemirror/state'

export type InsertKind =
  | 'h1'
  | 'h2'
  | 'h3'
  | 'bold'
  | 'italic'
  | 'code'
  | 'bullet'
  | 'numbered'
  | 'task'
  | 'quote'
  | 'codeblock'
  | 'table'
  | 'link'
  | 'synapse'
  | 'rule'

/** The mark a line starts with: heading, task, list item or quote. */
const LINE_MARK = /^(\s*)(#{1,6}\s+|[-*+]\s+\[[ xX]\]\s+|[-*+]\s+|\d+[.)]\s+|>\s?)?/

const linePrefix: Partial<Record<InsertKind, (n: number) => string>> = {
  h1: () => '# ',
  h2: () => '## ',
  h3: () => '### ',
  bullet: () => '- ',
  numbered: (n) => `${n}. `,
  task: () => '- [ ] ',
  quote: () => '> ',
}

const wrapMarks: Partial<Record<InsertKind, string>> = { bold: '**', italic: '*', code: '`' }

/** The change that puts this kind of Markdown at the selection. */
export function markdownInsert(state: EditorState, kind: InsertKind): TransactionSpec {
  const sel = state.selection.main
  const prefix = linePrefix[kind]
  if (prefix) return lineMarks(state, kind, prefix)
  const mark = wrapMarks[kind]
  if (mark) return wrap(state, mark, mark)
  const text = state.sliceDoc(sel.from, sel.to)
  switch (kind) {
    case 'link': {
      const label = text || 'text'
      const insert = `[${label}](url)`
      // The part still to fill in is selected: the address, or the text.
      const start = text ? sel.from + label.length + 3 : sel.from + 1
      const end = text ? start + 3 : start + label.length
      return { changes: { from: sel.from, to: sel.to, insert }, selection: EditorSelection.single(start, end) }
    }
    case 'synapse':
      return wrap(state, '[[', ']]')
    case 'codeblock':
      return block(state, text ? '```\n' + text + '\n```' : '```\n\n```', text ? null : 4)
    case 'table':
      return block(state, '| Column | Column |\n| --- | --- |\n|  |  |', 2, 8)
    case 'rule':
      return block(state, '---', null)
    default:
      return {}
  }
}

/** Sets (or, when every line has it already, removes) a line mark on the selected lines. */
function lineMarks(state: EditorState, kind: InsertKind, prefix: (n: number) => string): TransactionSpec {
  const sel = state.selection.main
  const first = state.doc.lineAt(sel.from).number
  const last = state.doc.lineAt(sel.to).number
  const lines = []
  for (let n = first; n <= last; n++) lines.push(state.doc.line(n))
  const same = (mark: string, n: number) => (kind === 'numbered' ? /^\d+[.)]\s+$/.test(mark) : mark === prefix(n))
  const marks = lines.map((l) => LINE_MARK.exec(l.text)!)
  const off = marks.every((m, i) => m[2] !== undefined && same(m[2], i + 1))
  const changes = lines.map((l, i) => {
    const m = marks[i]!
    const from = l.from + m[1]!.length
    return { from, to: from + (m[2]?.length ?? 0), insert: off ? '' : prefix(i + 1) }
  })
  return { changes, selection: state.selection.map(state.changes(changes)) }
}

/** Wraps the selection in marks; an empty selection leaves the cursor between them. */
function wrap(state: EditorState, open: string, close: string): TransactionSpec {
  const sel = state.selection.main
  const before = state.sliceDoc(sel.from - open.length, sel.from)
  const after = state.sliceDoc(sel.to, sel.to + close.length)
  // Wrapped already: the marks come off.
  if (!sel.empty && before === open && after === close) {
    return {
      changes: [
        { from: sel.from - open.length, to: sel.from },
        { from: sel.to, to: sel.to + close.length },
      ],
      selection: EditorSelection.single(sel.from - open.length, sel.to - open.length),
    }
  }
  return {
    changes: [
      { from: sel.from, insert: open },
      { from: sel.to, insert: close },
    ],
    selection: EditorSelection.single(sel.from + open.length, sel.to + open.length),
  }
}

/**
 * Puts a block on lines of its own (replacing the selection), with a blank
 * line around it where text touches it. cursor is the offset in the block to
 * put the cursor at (null: after it); selectTo selects from cursor to there.
 */
function block(state: EditorState, text: string, cursor: number | null, selectTo?: number): TransactionSpec {
  const sel = state.selection.main
  const lineBefore = state.doc.lineAt(sel.from)
  const lineAfter = state.doc.lineAt(sel.to)
  const textBefore = state.sliceDoc(lineBefore.from, sel.from)
  const textAfter = state.sliceDoc(sel.to, lineAfter.to)
  const lead = textBefore.trim() ? '\n\n' : ''
  const trail = textAfter.trim() ? '\n\n' : sel.to === state.doc.length ? '\n' : ''
  const insert = lead + text + trail
  const start = sel.from + lead.length
  const anchor = cursor === null ? start + text.length + (trail ? 1 : 0) : start + cursor
  const head = selectTo === undefined ? anchor : start + selectTo
  return { changes: { from: sel.from, to: sel.to, insert }, selection: EditorSelection.single(anchor, head) }
}
