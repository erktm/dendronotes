<script lang="ts" module>
  /** Data type used when dragging tree entries. */
  export const DRAG_TYPE = 'application/x-dendronotes-path'
</script>

<script lang="ts">
  import TreeItem from './TreeItem.svelte'
  import TreeInput from './TreeInput.svelte'
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { router } from '../lib/router.svelte'
  import { cssVars } from '../lib/actions'
  import { displayName, isNote } from '../lib/paths'
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
  let dropTarget = $state(false)

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
      app.edit = { kind: 'rename', path: node.path }
    } else if (e.key === 'Delete' || (e.key === 'Backspace' && e.metaKey)) {
      e.preventDefault()
      void app.remove(node.path)
    } else if (isDir && e.key === 'ArrowRight' && !open) {
      app.expanded.add(node.path)
    } else if (isDir && e.key === 'ArrowLeft' && open) {
      app.expanded.delete(node.path)
    }
  }

  function ondragstart(e: DragEvent) {
    if (!e.dataTransfer) return
    e.dataTransfer.setData(DRAG_TYPE, node.path)
    e.dataTransfer.effectAllowed = 'move'
  }

  // Folders accept drops on themselves and on everything inside them.
  function ondragover(e: DragEvent) {
    if (!isDir || !e.dataTransfer?.types.includes(DRAG_TYPE)) return
    e.preventDefault()
    e.stopPropagation()
    e.dataTransfer.dropEffect = 'move'
    dropTarget = true
  }

  function ondragleave(e: DragEvent) {
    const li = e.currentTarget as HTMLElement
    if (!li.contains(e.relatedTarget as Node | null)) dropTarget = false
  }

  function ondrop(e: DragEvent) {
    if (!isDir) return
    e.preventDefault()
    e.stopPropagation()
    dropTarget = false
    const from = e.dataTransfer?.getData(DRAG_TYPE)
    if (from) void app.moveInto(from, node.path)
  }
</script>

<li
  role="treeitem"
  aria-expanded={isDir ? open : undefined}
  aria-selected={active}
  class:drop={dropTarget}
  {ondragover}
  {ondragleave}
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
      class:muted={!isDir && !isNote(node.path)}
      use:cssVars={{ '--depth': depth }}
      draggable="true"
      title={node.path}
      onclick={activate}
      oncontextmenu={(e) => onmenu(e, node)}
      {onkeydown}
      {ondragstart}
    >
      {#if isDir}
        <span class="chevron" class:open><Icon name="chevron" size={14} /></span>
        <Icon name={open ? 'folder-open' : 'folder'} size={17} />
      {:else}
        <span class="chevron"></span>
        <Icon name={isNote(node.path) ? 'note' : 'file'} size={17} />
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
            kind={creating.kind === 'new-folder' ? 'folder' : 'note'}
            onsubmit={(name) => app.createEntry(creating.kind === 'new-folder' ? 'folder' : 'note', node.path, name)}
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
