<script lang="ts">
  import TreeItem, { DRAG_TYPE } from './TreeItem.svelte'
  import TreeInput from './TreeInput.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import { app } from '../lib/state/app.svelte'
  import { strings } from '../lib/strings'
  import type { TreeNode } from '../lib/tree'

  let menu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null)
  let rootDrop = $state(false)

  const creatingAtRoot = $derived(app.edit && app.edit.kind !== 'rename' && app.edit.parent === '' ? app.edit : null)
  const startCreate = (kind: 'new-note' | 'new-folder', parent: string) => app.startCreate(kind, parent)

  function openMenu(e: MouseEvent, node: TreeNode | null) {
    e.preventDefault()
    e.stopPropagation()
    if (!app.canEdit) return // every entry in the menu changes the soma
    // Keyboard-opened context menus report (0, 0): anchor at the element.
    let { clientX: x, clientY: y } = e
    if (x === 0 && y === 0 && e.currentTarget instanceof HTMLElement) {
      const r = e.currentTarget.getBoundingClientRect()
      x = r.left + 24
      y = r.bottom
    }
    const items: MenuItem[] = []
    const folder = node ? (node.type === 'dir' ? node.path : null) : ''
    if (folder !== null) {
      items.push(
        { label: strings.tree.newNote, icon: 'note', action: () => startCreate('new-note', folder) },
        { label: strings.tree.newFolder, icon: 'folder-plus', action: () => startCreate('new-folder', folder) },
      )
    }
    if (node) {
      items.push(
        { label: strings.tree.rename, icon: 'pencil', action: () => (app.edit = { kind: 'rename', path: node.path }) },
        {
          label: strings.tree.delete,
          icon: 'trash',
          danger: true,
          action: () => void app.remove(node.path),
        },
      )
    }
    menu = { x, y, items }
  }

  function ondragover(e: DragEvent) {
    if (!app.canEdit) return
    const types = e.dataTransfer?.types ?? []
    const files = types.includes('Files')
    if (!e.dataTransfer || (!types.includes(DRAG_TYPE) && !files)) return
    e.preventDefault()
    e.dataTransfer.dropEffect = files ? 'copy' : 'move'
    rootDrop = true
  }

  function ondrop(e: DragEvent) {
    e.preventDefault()
    rootDrop = false
    const files = [...(e.dataTransfer?.files ?? [])]
    if (files.length) {
      void app.uploadInto(files, '')
      return
    }
    const from = e.dataTransfer?.getData(DRAG_TYPE)
    if (from) void app.moveInto(from, '')
  }

  const empty = $derived(app.treeLoaded && app.tree.length === 0 && !creatingAtRoot)
</script>

<div
  class="tree"
  class:drop={rootDrop}
  role="presentation"
  oncontextmenu={(e) => openMenu(e, null)}
  {ondragover}
  ondragleave={(e) => {
    if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node | null)) rootDrop = false
  }}
  {ondrop}
>
  <ul role="tree" aria-label={strings.soma.treeLabel}>
    {#if creatingAtRoot}
      <li>
        <TreeInput
          depth={0}
          kind={creatingAtRoot.kind === 'new-folder' ? 'folder' : 'note'}
          onsubmit={(name) => app.createEntry(creatingAtRoot.kind === 'new-folder' ? 'folder' : 'note', '', name)}
        />
      </li>
    {/if}
    {#each app.tree as node (node.path)}
      <TreeItem {node} depth={0} onmenu={openMenu} />
    {/each}
  </ul>

  {#if empty}
    <div class="empty">
      <p>{strings.soma.empty}</p>
      {#if app.canEdit}
        <button type="button" onclick={() => startCreate('new-note', '')}>{strings.soma.createFirstNote}</button>
      {/if}
    </div>
  {:else if app.somaId && !app.treeLoaded}
    <p class="loading">{strings.soma.loading}</p>
  {/if}
</div>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />
{/if}

<style>
  .tree {
    flex: 1;
    overflow: auto;
    padding: var(--space-2) var(--space-2) var(--space-6);
    border-radius: 8px;
  }
  .tree.drop {
    box-shadow: inset 0 0 0 1.5px var(--accent);
    background: color-mix(in srgb, var(--accent-soft) 40%, transparent);
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .empty {
    padding: var(--space-4) var(--space-2);
    color: var(--text-faint);
    font-size: var(--text-sm);
  }
  .empty button {
    margin-top: var(--space-2);
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent);
    font-weight: 550;
    cursor: pointer;
  }
  .empty button:hover {
    text-decoration: underline;
  }
  .loading {
    padding: var(--space-4) var(--space-2);
    color: var(--text-faint);
    font-size: var(--text-sm);
  }
</style>
