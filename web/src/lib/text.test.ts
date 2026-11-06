import { describe, expect, it } from 'vitest'
import { diffLines } from './diff'
import { fuzzyMatch, highlightChunks } from './fuzzy'
import { outline } from './outline'

describe('diffLines', () => {
  it('marks added and removed lines', () => {
    const d = diffLines('a\nb\nc', 'a\nB\nc\nd')
    expect(d.map((l) => `${l.type}:${l.text}`)).toEqual(['same:a', 'del:b', 'add:B', 'same:c', 'add:d'])
  })

  it('handles identical and empty texts', () => {
    expect(diffLines('x\ny', 'x\ny').every((l) => l.type === 'same')).toBe(true)
    expect(diffLines('', 'new')).toEqual([
      { type: 'del', text: '' },
      { type: 'add', text: 'new' },
    ])
  })
})

describe('fuzzyMatch', () => {
  it('requires the characters in order', () => {
    expect(fuzzyMatch('gdn', 'Garden plan')).not.toBeNull()
    expect(fuzzyMatch('ndg', 'Garden plan')).toBeNull()
  })

  it('ranks prefixes and word starts higher', () => {
    const prefix = fuzzyMatch('gard', 'Garden plan')!.score
    const inner = fuzzyMatch('gard', 'Regards to the club')!.score
    expect(prefix).toBeGreaterThan(inner)
  })

  it('highlights matched characters', () => {
    const m = fuzzyMatch('gp', 'Garden plan')!
    expect(highlightChunks('Garden plan', m.indices)).toEqual([
      { text: 'G', hit: true },
      { text: 'arden ', hit: false },
      { text: 'p', hit: true },
      { text: 'lan', hit: false },
    ])
  })
})

describe('outline', () => {
  it('lists ATX headings outside code with unique slugs', () => {
    const src = '# Plan\ntext\n## Tasks\n```\n# not a heading\n```\n## Tasks\n### Café'
    expect(outline(src).map((h) => [h.level, h.text, h.line, h.slug])).toEqual([
      [1, 'Plan', 0, 'plan'],
      [2, 'Tasks', 2, 'tasks'],
      [2, 'Tasks', 6, 'tasks-1'],
      [3, 'Café', 7, 'cafe'],
    ])
  })
})
