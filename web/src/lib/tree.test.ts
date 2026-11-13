import { describe, expect, it } from 'vitest'
import type { Entry } from './api/soma'
import { buildTree, outermost, reorder, visiblePaths } from './tree'

const entries: Entry[] = [
  { path: 'Beds', type: 'dir', mtime: 50, ctime: 1 },
  { path: 'Beds/North.md', type: 'file', mtime: 30, ctime: 30 },
  { path: 'Beds/South.md', type: 'file', mtime: 40, ctime: 10 },
  { path: 'Beds/East.md', type: 'file', mtime: 20, ctime: 20 },
  { path: 'Archive', type: 'dir', mtime: 60, ctime: 2 },
  { path: 'Plan 10.md', type: 'file', mtime: 10, ctime: 5 },
  { path: 'Plan 9.md', type: 'file', mtime: 70 },
]
const names = (nodes: { name: string }[]) => nodes.map((n) => n.name)

describe('the tree', () => {
  it('lists folders first, in natural order by default', () => {
    const t = buildTree(entries)
    expect(names(t)).toEqual(['Archive', 'Beds', 'Plan 9.md', 'Plan 10.md'])
    expect(names(t[1]!.children)).toEqual(['East.md', 'North.md', 'South.md'])
  })

  it('sorts by name, last change or creation', () => {
    expect(names(buildTree(entries, {}, 'modified'))).toEqual(['Archive', 'Beds', 'Plan 9.md', 'Plan 10.md'])
    expect(names(buildTree(entries, {}, 'created')[1]!.children)).toEqual(['North.md', 'East.md', 'South.md'])
    // Without a creation time the last change stands in.
    expect(names(buildTree(entries, {}, 'created'))).toEqual(['Archive', 'Beds', 'Plan 9.md', 'Plan 10.md'])
  })

  it('keeps a manual order, with new entries after it by name', () => {
    const orders = { Beds: ['South.md', 'Gone.md', 'North.md'] }
    expect(names(buildTree(entries, orders, 'manual')[1]!.children)).toEqual(['South.md', 'North.md', 'East.md'])
    // Other modes leave the order alone; it is back when sorting by hand again.
    expect(names(buildTree(entries, orders, 'name')[1]!.children)).toEqual(['East.md', 'North.md', 'South.md'])
  })

  it('moves names in a manual order', () => {
    const now = ['a', 'b', 'c', 'd']
    expect(reorder(now, ['d'], 'b')).toEqual(['a', 'd', 'b', 'c'])
    expect(reorder(now, ['a', 'c'], null)).toEqual(['b', 'd', 'a', 'c'])
    expect(reorder(now, ['b'], 'b')).toEqual(['a', 'c', 'd', 'b'])
    expect(reorder(now, ['new'], 'a')).toEqual(['new', 'a', 'b', 'c', 'd'])
  })

  it('lists visible rows and the outermost of a selection', () => {
    const t = buildTree(entries)
    expect(visiblePaths(t, new Set())).toEqual(['Archive', 'Beds', 'Plan 9.md', 'Plan 10.md'])
    expect(visiblePaths(t, new Set(['Beds']))).toEqual(['Archive', 'Beds', 'Beds/East.md', 'Beds/North.md', 'Beds/South.md', 'Plan 9.md', 'Plan 10.md'])
    expect(outermost(['Beds/North.md', 'Beds', 'Plan 9.md', 'Bedside.md'])).toEqual(['Beds', 'Bedside.md', 'Plan 9.md'])
  })
})
