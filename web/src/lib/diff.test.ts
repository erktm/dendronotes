import { describe, expect, it } from 'vitest'
import { diffLines, stats, withContext, type Op } from './diff'

const show = (ops: Op[]) => ops.map((o) => (o.kind === 'same' ? ' ' : o.kind === 'add' ? '+' : '-') + o.text)

/** Applying the ops to a must give b. */
function check(a: string, b: string) {
  const ops = diffLines(a, b)
  const before = ops.filter((o) => o.kind !== 'add').map((o) => o.text)
  const after = ops.filter((o) => o.kind !== 'del').map((o) => o.text)
  expect(before.join('\n')).toBe(a.replace(/^\ufeff/, '').replace(/\r\n?/g, '\n'))
  expect(after.join('\n')).toBe(b.replace(/^\ufeff/, '').replace(/\r\n?/g, '\n'))
  return ops
}

describe('diffLines', () => {
  it('finds nothing in equal texts', () => {
    expect(stats(check('a\nb\n', 'a\nb\n'))).toEqual({ added: 0, deleted: 0 })
  })

  it('shows added, deleted and changed lines', () => {
    expect(show(check('beds\npaths\npond', 'beds\ntrees\npond\nfence'))).toEqual([' beds', '-paths', '+trees', ' pond', '+fence'])
    expect(show(check('', 'one'))).toEqual(['-', '+one'])
    expect(show(check('one\ntwo', ''))).toEqual(['-one', '-two', '+'])
  })

  it('ignores line breaks and the byte order mark', () => {
    expect(stats(check('\ufeffa\r\nb\r\n', 'a\nb\n'))).toEqual({ added: 0, deleted: 0 })
  })

  it('stays minimal in longer texts', () => {
    const a = Array.from({ length: 200 }, (_, i) => `line ${i}`)
    const b = [...a]
    b.splice(50, 2, 'new A')
    b.splice(150, 0, 'new B', 'new C')
    b.push('end')
    expect(stats(check(a.join('\n'), b.join('\n')))).toEqual({ added: 4, deleted: 2 })
  })

  it('handles texts with many changes', () => {
    const a = Array.from({ length: 300 }, (_, i) => `a${i % 7}`).join('\n')
    const b = Array.from({ length: 300 }, (_, i) => `a${i % 5}`).join('\n')
    check(a, b)
  })
})

describe('withContext', () => {
  it('folds unchanged lines far from changes', () => {
    const a = Array.from({ length: 20 }, (_, i) => `l${i}`)
    const b = [...a]
    b[10] = 'changed'
    const rows = withContext(diffLines(a.join('\n'), b.join('\n')), 2)
    expect(rows[0]).toEqual({ kind: 'skip', count: 8 })
    expect(rows.at(-1)).toEqual({ kind: 'skip', count: 7 })
    expect(rows.filter((r) => r.kind !== 'skip')).toHaveLength(6)
  })
})
