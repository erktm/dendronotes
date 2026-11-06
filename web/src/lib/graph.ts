// What the graph view shows: a selection of the soma's notes and synapses.

import type { GraphData } from './api/soma'
import { displayName } from './paths'

export interface GraphOptions {
  /** All notes, or the neighborhood of one note. */
  mode: 'global' | 'local'
  /** Steps from the center note in local mode (1–3). */
  depth: number
  /** Show notes that synapses point to but that do not exist yet. */
  missing: boolean
  /** Show notes without any synapse (global mode). */
  orphans: boolean
}

export interface ViewNode {
  id: string
  label: string
  missing: boolean
  tags: string[]
  /** Number of synapses in and out (within the selection). */
  degree: number
}

export interface ViewEdge {
  source: string
  target: string
}

export interface Selection {
  nodes: ViewNode[]
  edges: ViewEdge[]
}

/** The nodes and edges to draw for the given options; center is the open note. */
export function selectGraph(data: GraphData, opts: GraphOptions, center: string | null): Selection {
  const byId = new Map(data.nodes.map((n) => [n.id, n]))
  let ids = new Set(data.nodes.filter((n) => opts.missing || !n.missing).map((n) => n.id))
  let edges = data.edges.filter((e) => ids.has(e.source) && ids.has(e.target))

  if (opts.mode === 'local' && center && byId.has(center)) {
    const neighbors = new Map<string, string[]>()
    for (const e of edges) {
      neighbors.set(e.source, [...(neighbors.get(e.source) ?? []), e.target])
      neighbors.set(e.target, [...(neighbors.get(e.target) ?? []), e.source])
    }
    const keep = new Set([center])
    let frontier = [center]
    for (let step = 0; step < opts.depth; step++) {
      const next: string[] = []
      for (const id of frontier) {
        for (const n of neighbors.get(id) ?? []) {
          if (!keep.has(n)) {
            keep.add(n)
            next.push(n)
          }
        }
      }
      frontier = next
    }
    ids = keep
    edges = edges.filter((e) => ids.has(e.source) && ids.has(e.target))
  }

  const degree = new Map<string, number>()
  for (const e of edges) {
    degree.set(e.source, (degree.get(e.source) ?? 0) + 1)
    degree.set(e.target, (degree.get(e.target) ?? 0) + 1)
  }
  const nodes: ViewNode[] = []
  for (const id of ids) {
    const n = byId.get(id)!
    const d = degree.get(id) ?? 0
    if (opts.mode === 'global' && !opts.orphans && d === 0 && id !== center) continue
    nodes.push({ id, label: n.missing ? (n.label ?? id.slice(1)) : displayName(id), missing: !!n.missing, tags: n.tags ?? [], degree: d })
  }
  return { nodes, edges }
}

/** Radius of a node: grows slowly with its number of synapses. */
export function nodeRadius(degree: number): number {
  return Math.min(4 + Math.sqrt(degree) * 1.8, 14)
}
