// Comments are plain text. For showing them, @mentions and web addresses
// are picked out – the text itself is never inserted as HTML.

export type Part = { kind: 'text'; text: string } | { kind: 'mention'; text: string; name: string } | { kind: 'link'; text: string; href: string }

const nameChar = /[\p{L}\p{N}._@-]/u
const startChar = /[\p{L}\p{N}]/u
const url = /\bhttps?:\/\/[^\s<>"]+/giu

/** User names written as @name (the same rules as the server). */
export function mentionsIn(text: string): string[] {
  return parts(text)
    .filter((p): p is Extract<Part, { kind: 'mention' }> => p.kind === 'mention')
    .map((p) => p.name)
}

function mentionParts(text: string): Part[] {
  const out: Part[] = []
  const chars = [...text]
  let plain = ''
  for (let i = 0; i < chars.length; i++) {
    const c = chars[i] ?? ''
    const prev = chars[i - 1] ?? ''
    const next = chars[i + 1] ?? ''
    if (c === '@' && !nameChar.test(prev) && startChar.test(next)) {
      let j = i + 1
      while (j < chars.length && nameChar.test(chars[j] ?? '')) j++
      let name = chars.slice(i + 1, j).join('')
      const trimmed = name.replace(/[._@-]+$/u, '')
      j -= name.length - trimmed.length
      name = trimmed
      if (name.length >= 2) {
        if (plain) out.push({ kind: 'text', text: plain })
        plain = ''
        out.push({ kind: 'mention', text: '@' + name, name })
        i = j - 1
        continue
      }
    }
    plain += c
  }
  if (plain) out.push({ kind: 'text', text: plain })
  return out
}

/** The comment split into text, mentions and links. */
export function parts(text: string): Part[] {
  const out: Part[] = []
  let last = 0
  for (const m of text.matchAll(url)) {
    // Punctuation at the end belongs to the sentence, not the address.
    const href = m[0].replace(/[.,;:!?)\]]+$/u, '')
    const at = m.index ?? 0
    out.push(...mentionParts(text.slice(last, at)))
    out.push({ kind: 'link', text: href, href })
    last = at + href.length
  }
  out.push(...mentionParts(text.slice(last)))
  return out
}

/** The @word being typed at the cursor, for suggestions. */
export function mentionAt(text: string, cursor: number): { from: number; query: string } | null {
  const before = text.slice(0, cursor)
  const m = /(^|[^\p{L}\p{N}._@-])@([\p{L}\p{N}._@-]{0,64})$/u.exec(before)
  if (!m) return null
  const query = m[2] ?? ''
  return { from: cursor - query.length - 1, query }
}
