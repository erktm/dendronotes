import type { Entry } from './api/soma'
import { basename, dirname } from './paths'

export interface TreeNode {
  name: string
  path: string
  type: 'file' | 'dir'
  readOnly: boolean
  children: TreeNode[]
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

function compare(a: TreeNode, b: TreeNode): number {
  if (a.type !== b.type) return a.type === 'dir' ? -1 : 1
  return collator.compare(a.name, b.name)
}

/** Builds a sorted tree (folders first, natural order) from the flat list. */
export function buildTree(entries: Entry[]): TreeNode[] {
  const root: TreeNode[] = []
  const dirs = new Map<string, TreeNode>()
  // The server lists parents before their children.
  for (const e of entries) {
    const node: TreeNode = { name: basename(e.path), path: e.path, type: e.type, readOnly: !!e.readOnly, children: [] }
    const parent = dirname(e.path)
    const siblings = parent === '' ? root : dirs.get(parent)?.children
    if (!siblings) continue
    siblings.push(node)
    if (e.type === 'dir') dirs.set(e.path, node)
  }
  const sort = (nodes: TreeNode[]) => {
    nodes.sort(compare)
    for (const n of nodes) if (n.children.length) sort(n.children)
  }
  sort(root)
  return root
}
