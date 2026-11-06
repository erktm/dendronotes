// Fuzzy matching for the quick switcher: the query's characters must appear
// in order; consecutive runs and word starts score higher.

export interface FuzzyMatch {
  score: number
  /** Indices of the matched characters in the text. */
  indices: number[]
}

export function fuzzyMatch(query: string, text: string): FuzzyMatch | null {
  const q = query.toLowerCase().replace(/\s+/g, '')
  if (!q) return { score: 0, indices: [] }
  const t = text.toLowerCase()
  const indices: number[] = []
  let score = 0
  let ti = 0
  let prev = -2
  for (const ch of q) {
    const found = t.indexOf(ch, ti)
    if (found < 0) return null
    const wordStart = found === 0 || /[\s/_\-.]/.test(t[found - 1]!)
    score += 1
    if (found === prev + 1) score += 8
    if (wordStart) score += 6
    score -= Math.min(found - ti, 6) * 0.5
    indices.push(found)
    prev = found
    ti = found + 1
  }
  if (t.startsWith(q)) score += 10
  if (t === q) score += 20
  return { score: score - text.length * 0.01, indices }
}

/** Splits text into chunks marked as matched or not, for highlighting. */
export function highlightChunks(text: string, indices: number[]): { text: string; hit: boolean }[] {
  const set = new Set(indices)
  const out: { text: string; hit: boolean }[] = []
  for (let i = 0; i < text.length; i++) {
    const hit = set.has(i)
    const last = out[out.length - 1]
    if (last && last.hit === hit) last.text += text[i]
    else out.push({ text: text[i]!, hit })
  }
  return out
}
