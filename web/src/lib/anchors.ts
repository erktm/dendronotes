// Where a comment belongs in a note: the quoted text with a little context
// before and after it, found again in the current text after edits (the
// server stores it as it is, positions would go stale).

export interface Anchor {
  quote: string
  prefix: string
  suffix: string
}

export interface Span {
  from: number
  to: number
}

/** Characters of context kept on each side of a quote. */
export const CONTEXT = 32

/** The anchor for text[from, to). */
export function makeAnchor(text: string, from: number, to: number): Anchor {
  return {
    quote: text.slice(from, to),
    prefix: text.slice(Math.max(0, from - CONTEXT), from),
    suffix: text.slice(to, to + CONTEXT),
  }
}

/** How many characters before at agree with prefix (from the end), and after end with suffix. */
function score(text: string, at: number, end: number, a: { prefix: string; suffix: string }): number {
  let n = 0
  for (let i = 1; i <= a.prefix.length && at - i >= 0 && text[at - i] === a.prefix[a.prefix.length - i]; i++) n++
  for (let i = 0; i < a.suffix.length && end + i < text.length && text[end + i] === a.suffix[i]; i++) n++
  return n
}

/** Where the quote is now: the occurrence that fits its context best, or null if it is gone. */
export function findAnchor(text: string, a: Anchor): Span | null {
  if (!a.quote) return null
  let best: Span | null = null
  let bestScore = -1
  let found = 0
  for (let at = text.indexOf(a.quote); at !== -1 && found < 1000; at = text.indexOf(a.quote, at + 1)) {
    found++
    const s = score(text, at, at + a.quote.length, a)
    if (s > bestScore) {
      best = { from: at, to: at + a.quote.length }
      bestScore = s
    }
  }
  return best
}

/**
 * An anchor for text selected in the reading view: the selected words with
 * the rendered text around them, placed in the source of the note where they
 * fit best. Formatting (**bold**, links) can make the selection differ from
 * the source; then the anchor keeps the rendered context.
 */
export function anchorFromRendered(source: string, quote: string, before: string, after: string): Anchor {
  const q = quote.trim()
  const span = q ? findAnchor(source, { quote: q, prefix: before.slice(-CONTEXT), suffix: after.slice(0, CONTEXT) }) : null
  if (span) return makeAnchor(source, span.from, span.to)
  return { quote: q, prefix: before.slice(-CONTEXT), suffix: after.slice(0, CONTEXT) }
}
