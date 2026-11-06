import { MARK_END, MARK_START } from './api/soma'

export interface SnippetPart {
  text: string
  mark: boolean
}

/**
 * Splits a search snippet at its match markers. The parts are rendered as
 * text nodes, so a note's content can never inject markup here.
 */
export function snippetParts(snippet: string): SnippetPart[] {
  const out: SnippetPart[] = []
  snippet.split(MARK_START).forEach((chunk, i) => {
    const end = i === 0 ? -1 : chunk.indexOf(MARK_END)
    if (end < 0) {
      if (chunk) out.push({ text: chunk.replaceAll(MARK_END, ''), mark: false })
      return
    }
    if (end > 0) out.push({ text: chunk.slice(0, end), mark: true })
    const rest = chunk.slice(end + 1).replaceAll(MARK_END, '')
    if (rest) out.push({ text: rest, mark: false })
  })
  return out
}
