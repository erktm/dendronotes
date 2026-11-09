// Line differences between two versions of a note (Myers' algorithm), for
// the history. Line breaks and a byte order mark do not count.

export interface Op {
  kind: 'same' | 'add' | 'del'
  text: string
}

/** A run of unchanged lines left out between changes. */
export interface Skip {
  kind: 'skip'
  count: number
}

export type Row = Op | Skip

/** Above this many changed lines, the versions are shown as replaced as a whole. */
const MAX_EDITS = 4000

function lines(s: string): string[] {
  return s.replace(/^\ufeff/, '').replace(/\r\n?/g, '\n').split('\n')
}

/** The lines of b compared with those of a: kept, added and deleted. */
export function diffLines(a: string, b: string): Op[] {
  const x = lines(a)
  const y = lines(b)
  // Common start and end need no search.
  let start = 0
  while (start < x.length && start < y.length && x[start] === y[start]) start++
  let endX = x.length
  let endY = y.length
  while (endX > start && endY > start && x[endX - 1] === y[endY - 1]) {
    endX--
    endY--
  }
  const head: Op[] = x.slice(0, start).map((text) => ({ kind: 'same', text }))
  const tail: Op[] = x.slice(endX).map((text) => ({ kind: 'same', text }))
  return [...head, ...middle(x.slice(start, endX), y.slice(start, endY)), ...tail]
}

function middle(x: string[], y: string[]): Op[] {
  const n = x.length
  const m = y.length
  if (n === 0) return y.map((text) => ({ kind: 'add', text }))
  if (m === 0) return x.map((text) => ({ kind: 'del', text }))
  const max = Math.min(n + m, MAX_EDITS)
  const offset = max + 1
  // v[offset + k]: the furthest x reached on diagonal k (typed arrays read 0 past the end).
  const at = (v: Int32Array, k: number) => v[offset + k] ?? 0
  let v = new Int32Array(2 * max + 3)
  const trace: Int32Array[] = []
  for (let d = 0; d <= max; d++) {
    trace.push(v.slice())
    const next = v.slice()
    for (let k = -d; k <= d; k += 2) {
      let i = k === -d || (k !== d && at(v, k - 1) < at(v, k + 1)) ? at(v, k + 1) : at(v, k - 1) + 1
      let j = i - k
      while (i < n && j < m && x[i] === y[j]) {
        i++
        j++
      }
      next[offset + k] = i
      if (i >= n && j >= m) {
        trace.push(next)
        return backtrack(trace, x, y, at)
      }
    }
    v = next
  }
  // Too different: replaced as a whole.
  return [...x.map((text): Op => ({ kind: 'del', text })), ...y.map((text): Op => ({ kind: 'add', text }))]
}

function backtrack(trace: Int32Array[], x: string[], y: string[], at: (v: Int32Array, k: number) => number): Op[] {
  const out: Op[] = []
  let i = x.length
  let j = y.length
  const xi = (n: number) => x[n] ?? ''
  const yi = (n: number) => y[n] ?? ''
  // trace[d] holds the furthest points before edit d.
  for (let d = trace.length - 2; d >= 0; d--) {
    const v = trace[d] ?? new Int32Array(0)
    const k = i - j
    const prevK = k === -d || (k !== d && at(v, k - 1) < at(v, k + 1)) ? k + 1 : k - 1
    const prevI = d === 0 ? 0 : at(v, prevK)
    const prevJ = prevI - prevK
    while (i > prevI && j > prevJ) {
      out.push({ kind: 'same', text: xi(--i) })
      j--
    }
    if (d === 0) break
    if (i === prevI) out.push({ kind: 'add', text: yi(--j) })
    else out.push({ kind: 'del', text: xi(--i) })
  }
  while (i > 0 && j > 0) {
    out.push({ kind: 'same', text: xi(--i) })
    j--
  }
  return out.reverse()
}

/** Keeps context lines around changes and folds the rest into skips. */
export function withContext(ops: Op[], context = 3): Row[] {
  const keep = new Uint8Array(ops.length)
  ops.forEach((op, i) => {
    if (op.kind === 'same') return
    for (let j = Math.max(0, i - context); j <= Math.min(ops.length - 1, i + context); j++) keep[j] = 1
  })
  const out: Row[] = []
  let skipped = 0
  ops.forEach((op, i) => {
    if (keep[i]) {
      if (skipped) out.push({ kind: 'skip', count: skipped })
      skipped = 0
      out.push(op)
    } else skipped++
  })
  if (skipped) out.push({ kind: 'skip', count: skipped })
  return out
}

/** Numbers of added and deleted lines. */
export function stats(ops: Op[]): { added: number; deleted: number } {
  let added = 0
  let deleted = 0
  for (const op of ops) {
    if (op.kind === 'add') added++
    else if (op.kind === 'del') deleted++
  }
  return { added, deleted }
}
