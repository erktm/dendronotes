import { describe, expect, it } from 'vitest'
import { mentionAt, mentionsIn, parts } from './commentText'

describe('comment text', () => {
  it('finds mentions like the server', () => {
    const cases: Record<string, string[]> = {
      '@ben': ['ben'],
      'hi @ben, and @cara.': ['ben', 'cara'],
      'mail me@example.org': [],
      '@erik@corp.example please': ['erik@corp.example'],
      '(@dora-lee)': ['dora-lee'],
      '@@x': [],
      '@a only': [],
      '@über_alles': ['über_alles'],
    }
    for (const [text, want] of Object.entries(cases)) expect(mentionsIn(text), text).toEqual(want)
  })

  it('picks out web addresses, never other schemes', () => {
    expect(parts('See https://example.org/a?b=1. And javascript:alert(1) @ben')).toEqual([
      { kind: 'text', text: 'See ' },
      { kind: 'link', text: 'https://example.org/a?b=1', href: 'https://example.org/a?b=1' },
      { kind: 'text', text: '. And javascript:alert(1) ' },
      { kind: 'mention', text: '@ben', name: 'ben' },
    ])
    expect(parts('<b>x</b>')).toEqual([{ kind: 'text', text: '<b>x</b>' }])
  })

  it('knows the mention being typed', () => {
    expect(mentionAt('Hi @be', 6)).toEqual({ from: 3, query: 'be' })
    expect(mentionAt('Hi @', 4)).toEqual({ from: 3, query: '' })
    expect(mentionAt('me@ex', 5)).toBeNull()
    expect(mentionAt('Hi @ben there', 13)).toBeNull()
  })
})
