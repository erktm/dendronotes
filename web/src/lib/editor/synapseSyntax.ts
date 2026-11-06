// Lezer Markdown extension: parses [[…]] and ![[…]] as a Synapse node with
// SynapseMark children for the brackets, so the editor can style and hide them.

import type { InlineContext, MarkdownConfig } from '@lezer/markdown'
import { tags } from '@lezer/highlight'

const BANG = 33
const OPEN = 91
const CLOSE = 93
const NEWLINE = 10

export const SynapseSyntax: MarkdownConfig = {
  defineNodes: [
    { name: 'Synapse', style: tags.link },
    { name: 'SynapseMark', style: tags.processingInstruction },
  ],
  parseInline: [
    {
      name: 'Synapse',
      before: 'Link',
      parse(cx: InlineContext, next: number, start: number): number {
        let pos = start
        if (next === BANG) pos++
        else if (next !== OPEN) return -1
        if (cx.char(pos) !== OPEN || cx.char(pos + 1) !== OPEN) return -1
        const innerFrom = pos + 2
        for (let i = innerFrom; i < cx.end - 1; i++) {
          const c = cx.char(i)
          if (c === NEWLINE || c === OPEN) return -1
          if (c === CLOSE) {
            if (cx.char(i + 1) !== CLOSE || i === innerFrom) return -1
            return cx.addElement(
              cx.elt('Synapse', start, i + 2, [cx.elt('SynapseMark', start, innerFrom), cx.elt('SynapseMark', i, i + 2)]),
            )
          }
        }
        return -1
      },
    },
  ],
}
