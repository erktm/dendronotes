// Shows where the others in a note are: their selections and a caret with
// their name. Colors come from a fixed palette of classes (peer-c0 … peer-c7
// in styles/editor.css), so no inline styles are needed (CSP).

import { StateEffect, StateField, type Range } from '@codemirror/state'
import { Decoration, EditorView, WidgetType, type DecorationSet } from '@codemirror/view'

export interface PeerCursor {
  id: string
  name: string
  color: number
  anchor: number
  head: number
}

/** Replaces the cursors shown (positions in the current document). */
export const setPeers = StateEffect.define<PeerCursor[]>()

const colorClass = (c: number) => `peer-c${((c % 8) + 8) % 8}`

class Caret extends WidgetType {
  constructor(
    readonly name: string,
    readonly color: number,
  ) {
    super()
  }

  override eq(other: Caret): boolean {
    return other.name === this.name && other.color === this.color
  }

  toDOM(): HTMLElement {
    const caret = document.createElement('span')
    caret.className = `cm-peer-caret ${colorClass(this.color)}`
    caret.setAttribute('aria-hidden', 'true')
    const label = document.createElement('span')
    label.className = 'cm-peer-name'
    label.textContent = this.name // untrusted: text only
    caret.append(label)
    return caret
  }

  override ignoreEvent(): boolean {
    return true
  }
}

function build(peers: readonly PeerCursor[], length: number): DecorationSet {
  const ranges: Range<Decoration>[] = []
  for (const p of peers) {
    const anchor = Math.min(Math.max(p.anchor, 0), length)
    const head = Math.min(Math.max(p.head, 0), length)
    const from = Math.min(anchor, head)
    const to = Math.max(anchor, head)
    if (from < to) ranges.push(Decoration.mark({ class: `cm-peer-sel ${colorClass(p.color)}` }).range(from, to))
    ranges.push(Decoration.widget({ widget: new Caret(p.name, p.color), side: 1 }).range(head))
  }
  return Decoration.set(ranges, true)
}

const peersField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    deco = deco.map(tr.changes) // cursors move with the text
    for (const e of tr.effects) if (e.is(setPeers)) deco = build(e.value, tr.state.doc.length)
    return deco
  },
  provide: (f) => EditorView.decorations.from(f),
})

export const presence = [peersField]
