import type { Entry } from './api/soma'
import { basename, dirname } from './paths'

export interface TreeNode {
  name: string
  path: string
  type: 'file' | 'dir'
  readOnly: boolean
  /** Last change and creation (Unix milliseconds). */
  mtime: number
  ctime: number
  children: TreeNode[]
}

/** How a folder's entries are sorted; folders always come first. */
export type SortMode = 'name' | 'modified' | 'created' | 'manual'
export const SORT_MODES: readonly SortMode[] = ['name', 'modified', 'created', 'manual']

/** A folder's sort order; in manual mode names lists its entries in order. */
export interface FolderOrder {
  mode: SortMode
  names?: string[]
}

/** Sort orders by folder path ("" is the top level); folders not listed sort by name. */
export type Orders = Record<string, FolderOrder>

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

/** Sorts the entries of one folder (in place) the way its order says. */
export function sortNodes(nodes: TreeNode[], order: FolderOrder | undefined): TreeNode[] {
  const mode = order?.mode ?? 'name'
  const rank = new Map((mode === 'manual' ? (order?.names ?? []) : []).map((n, i) => [n, i]))
  const byName = (a: TreeNode, b: TreeNode) => collator.compare(a.name, b.name)
  return nodes.sort((a, b) => {
    if (a.type !== b.type) return a.type === 'dir' ? -1 : 1
    switch (mode) {
      case 'modified':
        return b.mtime - a.mtime || byName(a, b)
      case 'created':
        return b.ctime - a.ctime || byName(a, b)
      case 'manual': {
        // Entries the order does not know yet follow, by name.
        const ra = rank.get(a.name) ?? Infinity
        const rb = rank.get(b.name) ?? Infinity
        return ra === rb ? byName(a, b) : ra < rb ? -1 : 1
      }
      default:
        return byName(a, b)
    }
  })
}

/** Builds the sorted tree (folders first, each folder in its order) from the flat list. */
export function buildTree(entries: Entry[], orders: Orders = {}): TreeNode[] {
  const root: TreeNode[] = []
  const dirs = new Map<string, TreeNode>()
  // The server lists parents before their children.
  for (const e of entries) {
    const node: TreeNode = {
      name: basename(e.path),
      path: e.path,
      type: e.type,
      readOnly: !!e.readOnly,
      mtime: e.mtime,
      ctime: e.ctime ?? e.mtime,
      children: [],
    }
    const parent = dirname(e.path)
    const siblings = parent === '' ? root : dirs.get(parent)?.children
    if (!siblings) continue
    siblings.push(node)
    if (e.type === 'dir') dirs.set(e.path, node)
  }
  const sort = (nodes: TreeNode[], folder: string) => {
    sortNodes(nodes, orders[folder])
    for (const n of nodes) if (n.children.length) sort(n.children, n.path)
  }
  sort(root, '')
  return root
}

/**
 * The names of a folder in a new manual order: the moving names taken out
 * and put in front of `before` (at the end when it is null or moving itself).
 */
export function reorder(current: readonly string[], moving: readonly string[], before: string | null): string[] {
  const set = new Set(moving)
  const rest = current.filter((n) => !set.has(n))
  const at = before === null || set.has(before) ? -1 : rest.indexOf(before)
  const kept = moving.filter((n) => !rest.includes(n))
  if (at < 0) return [...rest, ...kept]
  return [...rest.slice(0, at), ...kept, ...rest.slice(at)]
}

/** The paths of the rows the tree shows, top to bottom (open folders only). */
export function visiblePaths(nodes: readonly TreeNode[], open: ReadonlySet<string>): string[] {
  const out: string[] = []
  const walk = (list: readonly TreeNode[]) => {
    for (const n of list) {
      out.push(n.path)
      if (n.type === 'dir' && open.has(n.path)) walk(n.children)
    }
  }
  walk(nodes)
  return out
}

/** The outermost of some paths: a folder's own entries go along with it. */
export function outermost(paths: Iterable<string>): string[] {
  const all = [...new Set(paths)].sort()
  return all.filter((p) => !all.some((q) => q !== p && p.startsWith(q + '/')))
}
