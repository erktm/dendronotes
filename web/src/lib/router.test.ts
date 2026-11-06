// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { noteURL, parse } from './router.svelte'

describe('router', () => {
  it('reads soma, note, graph and admin URLs', () => {
    expect(parse('/s/k3xq7m2a9b/n/Projects/Garden%20plan.md')).toEqual({
      soma: 'k3xq7m2a9b',
      notePath: 'Projects/Garden plan.md',
      view: 'soma',
      adminTab: '',
    })
    expect(parse('/s/k3xq7m2a9b')).toMatchObject({ soma: 'k3xq7m2a9b', notePath: null, view: 'soma' })
    expect(parse('/s/k3xq7m2a9b/graph')).toMatchObject({ soma: 'k3xq7m2a9b', view: 'graph' })
    expect(parse('/admin')).toMatchObject({ view: 'admin', adminTab: 'users' })
    expect(parse('/admin/audit')).toMatchObject({ view: 'admin', adminTab: 'audit' })
  })

  it('ignores what it does not know', () => {
    for (const p of ['/', '/n/Old.md', '/s/UPPER/n/x.md', '/s/a b/n/x.md', '/s/abc/n/%E0%A4%A.md']) {
      expect(parse(p).notePath).toBeNull()
    }
  })

  it('builds note URLs that parse back', () => {
    const url = noteURL('abc123', 'Ünïcode/50% done #1.md')
    expect(parse(url).notePath).toBe('Ünïcode/50% done #1.md')
  })
})
