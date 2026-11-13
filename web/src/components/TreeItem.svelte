<script lang="ts" module>
  /** Data type used when dragging tree entries (paths, one per line). */
  export const DRAG_TYPE = 'application/x-dendronotes-path'

  /** The paths a drop of tree entries carries. */
  export function droppedPaths(e: DragEvent): string[] {
    const data = e.dataTransfer?.getData(DRAG_TYPE) ?? ''
    return data ? data.split('\n').filter(Boolean) : []
  }
</script>

<script lang="ts">
  import TreeItem from './TreeItem.svelte'
  import TreeInput from './TreeInput.svelte'
  import Icon from './Icon.svelte'
  import { app, entryKind } from '../lib/state/app.svelte'
  import { isDrawingPath } from '../lib/drawing/paths'
  import { router } from '../lib/router.svelte'
  import { cssVars } from '../lib/actions'
  import { dirname, displayName, isNote } from '../lib/paths'
  import { droppedEntries, hasFolder } from '../lib/dropped'
  import { outermost } from '../lib/tree'
  import { treeDrag } from '../lib/treeDrag.svelte'
  import { strings } from '../lib/strings'
  import type { TreeNode } from '../lib/tree'

  let {
    node,
    depth,
    onmenu,
  }: { node: TreeNode; depth: number; onmenu: (e: MouseEvent, node: TreeNode) => void } = $props()

  const isDir = $derived(node.type === 'dir')
  const open = $derived(isDir && app.expanded.has(node.path))
  const active = $derived(router.notePath === node.path)
  const renaming = $derived(app.edit?.kind === 'rename' && app.edit.path === node.path)
  const creating = $derived(app.edit && app.edit.kind !== 'rename' && app.edit.parent === node.path ? app.edit : null)
  const selected = $derived(app.selected.has(node.path))
  // Where a drop would land: this folder, or before or after this row.
  const dropTarget = $derived(isDir && treeDrag.into === node.path)
  const place = $derived(treeDrag.place?.path === node.path ? treeDrag.place.at : null)
  const parent = $derived(dirname(node.path))

  function onclick(e: MouseEvent) {
    if (app.can.organize && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      app.toggleSelected(node.path)
      return
    }
    if (app.can.organize && e.shiftKey) {
      e.preventDefault()
      app.selectRange(node.path)
      return
    }
    if (app.selected.size) app.clearSelection()
    activate()
  }

  function activate() {
    if (isDir) {
      if (open) app.expanded.delete(node.path)
      else app.expanded.add(node.path)
    } else {
      void app.navigate(node.path)
    }
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'F2') {
      e.preventDefault()
      if (app.can.organize) app.edit = { kind: 'rename', path: node.path }
    } else if (e.key === 'Delete' || (e.key === 'Backspace' && e.metaKey)) {
      e.preventDefault()
      if (selected && app.selected.size > 1) void app.removeMany(app.selected)
      else void app.remove(node.path)
    } else if (e.key === 'Escape' && app.selected.size) {
      e.preventDefault()
      app.clearSelection()
    } else if (isDir && e.key === 'ArrowRight' && !open) {
      app.expanded.add(node.path)
    } else if (isDir && e.key === 'ArrowLeft' && open) {
      app.expanded.delete(node.path)
    }
  }

  function ondragstart(e: DragEvent) {
    if (!e.dataTransfer) return
    // A selected entry takes the whole selection along.
    treeDrag.paths = selected && app.selected.size > 1 ? outermost(app.selected) : [node.path]
    e.dataTransfer.setData(DRAG_TYPE, treeDrag.paths.join('\n'))
    e.dataTransfer.effectAllowed = 'move'
  }

  function ondragend() {
    treeDrag.reset()
  }

  /**
   * Sorted by hand, entries dragged over a row go before or after it (the
   * upper or lower part); a folder's middle still takes them in.
   */
  function placeAt(e: DragEvent): 'before' | 'after' | null {
    const moving = treeDrag.paths
    if (!app.can.organize || app.sortMode !== 'manual' || moving.length === 0 || moving.includes(node.path)) return null
    const row = (e.currentTarget as HTMLElement).querySelector(':scope > .row')
    if (!row) return null
    const r = row.getBoundingClientRect()
    if (e.clientY < r.top || e.clientY > r.bottom) return null
    const y = (e.clientY - r.top) / r.height
    // Below an open folder come its own entries, so its lower part takes entries in.
    if (isDir) return y < 0.25 ? 'before' : y > 0.75 && !open ? 'after' : null
    return y < 0.5 ? 'before' : 'after'
  }

  // Folders accept drops on themselves and on everything inside them:
  // entries of the tree (moved) and files and folders from the computer
  // (uploaded). Notes pass drops on to their folder, unless sorted by hand.
  function ondragover(e: DragEvent) {
    const types = e.dataTransfer?.types ?? []
    const files = types.includes('Files')
    if (!e.dataTransfer || !(files ? app.can.create : types.includes(DRAG_TYPE) && app.can.organize)) return
    const at = files ? null : placeAt(e)
    if (at) {
      e.preventDefault()
      e.stopPropagation()
      e.dataTransfer.dropEffect = 'move'
      treeDrag.show(null, { path: node.path, at })
      return
    }
    if (!isDir) return
    e.preventDefault()
    e.stopPropagation()
    e.dataTransfer.dropEffect = files ? 'copy' : 'move'
    treeDrag.show(node.path, null)
  }

  function ondrop(e: DragEvent) {
    const at = place
    treeDrag.clear()
    if (at) {
      e.preventDefault()
      e.stopPropagation()
      const paths = droppedPaths(e)
      // In front of the next entry that does not move itself.
      const siblings = app.childrenOf(parent).map((n) => n.name)
      const moving = new Set(paths.map((p) => p.split('/').pop()!))
      const before = at === 'before' ? node.name : (siblings.slice(siblings.indexOf(node.name) + 1).find((n) => !moving.has(n)) ?? null)
      void app.dropInOrder(paths, parent, before)
      return
    }
    if (!isDir) return
    e.preventDefault()
    e.stopPropagation()
    const entries = droppedEntries(e.dataTransfer)
    if (hasFolder(entries)) {
      void app.uploadDropped(entries, node.path)
      return
    }
    const files = [...(e.dataTransfer?.files ?? [])]
    if (files.length) {
      void app.uploadInto(files, node.path)
      return
    }
    const paths = droppedPaths(e)
    if (paths.length) void app.moveMany(paths, node.path)
  }
</script>

<li
  role="treeitem"
  aria-expanded={isDir ? open : undefined}
  aria-selected={active || selected}
  class:drop={dropTarget}
  class:place-before={place === 'before'}
  class:place-after={place === 'after'}
  {ondragover}
  {ondrop}
>
  {#if renaming}
    <TreeInput
      {depth}
      kind={isDir ? 'folder' : 'note'}
      initial={displayName(node.path)}
      onsubmit={(name) => app.rename(node.path, name)}
    />
  {:else}
    <button
      type="button"
      class="row"
      class:active
      class:selected
      class:muted={!isDir && !isNote(node.path)}
      use:cssVars={{ '--depth': depth }}
      draggable={app.can.organize ? 'true' : 'false'}
      title={node.path}
      {onclick}
      oncontextmenu={(e) => onmenu(e, node)}
      {onkeydown}
      {ondragstart}
      {ondragend}
    >
      {#if isDir}
        <span class="chevron" class:open><Icon name="chevron" size={14} /></span>
        <Icon name={open ? 'folder-open' : 'folder'} size={17} />
      {:else}
        <span class="chevron"></span>
        <Icon name={isNote(node.path) ? 'note' : isDrawingPath(node.path) ? 'drawing' : 'file'} size={17} />
      {/if}
      <span class="label">{displayName(node.path)}</span>
      {#if node.readOnly}<span class="badge" title={strings.tree.readOnly}><Icon name="lock" size={13} /></span>{/if}
    </button>
  {/if}

  {#if isDir && open}
    <ul role="group">
      {#if creating}
        <li>
          <TreeInput
            depth={depth + 1}
            kind={entryKind(creating.kind)}
            onsubmit={(name) => app.createEntry(entryKind(creating.kind), node.path, name)}
          />
        </li>
      {/if}
      {#each node.children as child (child.path)}
        <TreeItem node={child} depth={depth + 1} {onmenu} />
      {/each}
    </ul>
  {/if}
</li>

<style>
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    height: 30px;
    padding: 0 var(--space-2) 0 calc(var(--space-2) + var(--depth) * 14px);
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text-muted);
    font-size: var(--text-sm);
    text-align: left;
    cursor: pointer;
  }
  .row:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .row:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: -2px;
  }
  .row.active {
    background: var(--accent-soft);
    color: var(--text);
    font-weight: 550;
  }
  .row.active :global(.icon) {
    color: var(--accent);
  }
  .row.muted .label {
    color: var(--text-faint);
  }
  .row.selected {
    background: var(--accent-soft);
    box-shadow: inset 3px 0 0 var(--accent);
    color: var(--text);
  }
  /* Where an entry lands in a folder sorted by hand. */
  .place-before > .row,
  .place-after > .row {
    position: relative;
  }
  .place-before > .row::after,
  .place-after > .row::after {
    position: absolute;
    right: 4px;
    left: calc(var(--space-2) + var(--depth) * 14px);
    height: 2px;
    border-radius: 1px;
    background: var(--accent);
    content: '';
  }
  .place-before > .row::after {
    top: -1px;
  }
  .place-after > .row::after {
    bottom: -1px;
  }
  .drop > .row {
    background: var(--accent-soft);
    box-shadow: inset 0 0 0 1.5px var(--accent);
  }
  .chevron {
    display: grid;
    place-items: center;
    width: 14px;
    color: var(--text-faint);
    transition: transform 0.12s ease;
  }
  .chevron.open {
    transform: rotate(90deg);
  }
  .label {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .badge {
    display: grid;
    color: var(--text-faint);
  }
</style>
