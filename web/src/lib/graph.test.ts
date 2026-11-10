import { describe, expect, it } from 'vitest'
import { counts, selectGraph, type GraphOptions } from './graph'
import type { GraphData } from './api/soma'

const data: GraphData = {
  nodes: [
    { id: 'A.md' },
    { id: 'B.md', tags: ['x'] },
    { id: 'C.md' },
    { id: 'D.md' },
    { id: 'Lonely.md' },
    { id: '?ghost', label: 'Ghost', missing: true },
  ],
  edges: [
    { source: 'A.md', target: 'B.md' },
    { source: 'B.md', target: 'C.md' },
    { source: 'C.md', target: 'D.md' },
    { source: 'A.md', target: '?ghost' },
  ],
}
const all: GraphOptions = { mode: 'global', depth: 1, missing: true, orphans: true, drawings: true }
const ids = (opts: Partial<GraphOptions>, center: string | null = null) =>
  selectGraph(data, { ...all, ...opts }, center)
    .nodes.map((n) => n.id)
    .sort()

describe('selectGraph', () => {
  it('shows drawings with the notes embedding them, apart from synapses', () => {
    const withDrawings: GraphData = {
      ...data,
      nodes: [...data.nodes, { id: 'Plans/Beds.drawing.svg', kind: 'drawing' }, { id: 'Unused.drawing.svg', kind: 'drawing' }],
      embeds: [{ source: 'A.md', target: 'Plans/Beds.drawing.svg' }],
    }
    const s = selectGraph(withDrawings, all, null)
    expect(s.nodes.find((n) => n.id === 'Plans/Beds.drawing.svg')).toMatchObject({ label: 'Beds', drawing: true, degree: 1 })
    expect(s.nodes.find((n) => n.id === 'A.md')!.degree).toBe(3)
    expect(s.edges.filter((e) => e.embed)).toEqual([{ source: 'A.md', target: 'Plans/Beds.drawing.svg', embed: true }])
    expect(counts(s)).toEqual({ notes: 6, drawings: 2, synapses: 4 })
    // Without drawings, or without what stands alone.
    expect(selectGraph(withDrawings, { ...all, drawings: false }, null).nodes.some((n) => n.drawing)).toBe(false)
    expect(selectGraph(withDrawings, { ...all, orphans: false }, null).nodes.map((n) => n.id)).not.toContain('Unused.drawing.svg')
  })

  it('shows everything globally, with degrees and labels', () => {
    const s = selectGraph(data, all, null)
    expect(s.nodes).toHaveLength(6)
    expect(s.edges).toHaveLength(4)
    expect(s.nodes.find((n) => n.id === 'A.md')).toMatchObject({ label: 'A', degree: 2 })
    expect(s.nodes.find((n) => n.id === '?ghost')).toMatchObject({ label: 'Ghost', missing: true, degree: 1 })
    expect(s.nodes.find((n) => n.id === 'B.md')!.tags).toEqual(['x'])
  })

  it('hides missing notes and orphans on request', () => {
    expect(ids({ missing: false })).toEqual(['A.md', 'B.md', 'C.md', 'D.md', 'Lonely.md'])
    expect(ids({ orphans: false })).toEqual(['?ghost', 'A.md', 'B.md', 'C.md', 'D.md'])
    // The open note stays even without synapses.
    expect(ids({ orphans: false }, 'Lonely.md')).toContain('Lonely.md')
  })

  it('takes the neighborhood in both directions up to the depth', () => {
    expect(ids({ mode: 'local', depth: 1 }, 'B.md')).toEqual(['A.md', 'B.md', 'C.md'])
    expect(ids({ mode: 'local', depth: 2 }, 'B.md')).toEqual(['?ghost', 'A.md', 'B.md', 'C.md', 'D.md'])
    expect(ids({ mode: 'local', depth: 2, missing: false }, 'B.md')).toEqual(['A.md', 'B.md', 'C.md', 'D.md'])
    expect(ids({ mode: 'local', depth: 3 }, 'Lonely.md')).toEqual(['Lonely.md'])
    const s = selectGraph(data, { ...all, mode: 'local', depth: 1 }, 'B.md')
    expect(s.edges).toEqual([
      { source: 'A.md', target: 'B.md' },
      { source: 'B.md', target: 'C.md' },
    ])
  })

  it('falls back to the global graph without a center', () => {
    expect(ids({ mode: 'local' }, null)).toHaveLength(6)
    expect(ids({ mode: 'local' }, 'Unknown.md')).toHaveLength(6)
  })
})
