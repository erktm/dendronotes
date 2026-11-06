// Line diff for the conflict view (longest common subsequence).

export interface DiffLine {
  type: 'same' | 'add' | 'del'
  text: string
}

/** Above this many cells the diff falls back to "everything changed". */
const MAX_CELLS = 4_000_000

/** Diffs two texts line by line: what turns a into b. */
export function diffLines(a: string, b: string): DiffLine[] {
  const x = a.split('\n')
  const y = b.split('\n')

  // Common prefix and suffix are cheap and keep the table small.
  let start = 0
  while (start < x.length && start < y.length && x[start] === y[start]) start++
  let endX = x.length
  let endY = y.length
  while (endX > start && endY > start && x[endX - 1] === y[endY - 1]) {
    endX--
    endY--
  }
  const head = x.slice(0, start).map((text): DiffLine => ({ type: 'same', text }))
  const tail = x.slice(endX).map((text): DiffLine => ({ type: 'same', text }))
  const xs = x.slice(start, endX)
  const ys = y.slice(start, endY)

  let middle: DiffLine[]
  if (xs.length * ys.length > MAX_CELLS) {
    middle = [...xs.map((text): DiffLine => ({ type: 'del', text })), ...ys.map((text): DiffLine => ({ type: 'add', text }))]
  } else {
    middle = lcs(xs, ys)
  }
  return [...head, ...middle, ...tail]
}

function lcs(x: string[], y: string[]): DiffLine[] {
  const n = x.length
  const m = y.length
  const table = new Uint32Array((n + 1) * (m + 1))
  const at = (i: number, j: number) => i * (m + 1) + j
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      table[at(i, j)] = x[i] === y[j] ? table[at(i + 1, j + 1)]! + 1 : Math.max(table[at(i + 1, j)]!, table[at(i, j + 1)]!)
    }
  }
  const out: DiffLine[] = []
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (x[i] === y[j]) {
      out.push({ type: 'same', text: x[i]! })
      i++
      j++
    } else if (table[at(i + 1, j)]! >= table[at(i, j + 1)]!) {
      out.push({ type: 'del', text: x[i++]! })
    } else {
      out.push({ type: 'add', text: y[j++]! })
    }
  }
  while (i < n) out.push({ type: 'del', text: x[i++]! })
  while (j < m) out.push({ type: 'add', text: y[j++]! })
  return out
}
