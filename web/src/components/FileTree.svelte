<script lang="ts">
  import TreeItem, { DRAG_TYPE, droppedPaths } from './TreeItem.svelte'
  import TreeInput from './TreeInput.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import MoveDialog from './MoveDialog.svelte'
  import Icon from './Icon.svelte'
  import { app, entryKind, type NewEntryKind } from '../lib/state/app.svelte'
  import { droppedEntries, hasFolder } from '../lib/dropped'
  import { strings } from '../lib/strings'
  import { SORT_MODES, type TreeNode } from '../lib/tree'

  let menu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null)
  let rootDrop = $state(false)
  /** The entries the "Move to" dialog moves. */
  let moving = $state<string[] | null>(null)

  const creatingAtRoot = $derived(app.edit && app.edit.kind !== 'rename' && app.edit.parent === '' ? app.edit : null)
  const startCreate = (kind: NewEntryKind, parent: string) => app.startCreate(kind, parent)

  function openMenu(e: MouseEvent, node: TreeNode | null) {
    e.preventDefault()
    e.stopPropagation()
    // Keyboard-opened context menus report (0, 0): anchor at the element.
    let { clientX: x, clientY: y } = e
    if (x === 0 && y === 0 && e.currentTarget instanceof HTMLElement) {
      const r = e.currentTarget.getBoundingClientRect()
      x = r.left + 24
      y = r.bottom
    }
    const items: MenuItem[] = []
    // On a selected entry the menu is about the whole selection.
    if (node && app.can.organize && app.selected.has(node.path) && app.selected.size > 1) {
      const paths = [...app.selected]
      items.push(
        { label: strings.tree.moveTo, icon: 'folder-open', action: () => (moving = paths) },
        { label: strings.tree.deleteMany(paths.length), icon: 'trash', danger: true, action: () => void app.removeMany(paths) },
      )
      menu = { x, y, items }
      return
    }
    const folder = node ? (node.type === 'dir' ? node.path : null) : ''
    if (folder !== null && app.can.create) {
      items.push(
        { label: strings.tree.newNote, icon: 'note', action: () => startCreate('new-note', folder) },
        { label: strings.tree.newDrawing, icon: 'drawing', action: () => startCreate('new-drawing', folder) },
        { label: strings.tree.newFolder, icon: 'folder-plus', action: () => startCreate('new-folder', folder) },
      )
    }
    if (node && app.can.organize) {
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
    if (folder !== null && app.can.organize) {
      const mode = app.orderOf(folder).mode
      items.push(
        ...SORT_MODES.map((m, i) => ({
          label: strings.tree.sortModes[m]!,
          icon: 'check' as const,
          checked: mode === m,
          separator: i === 0 && items.length > 0,
          action: () => void app.setSort(folder, m),
        })),
      )
    }
    if (items.length) menu = { x, y, items } // every entry changes the soma
  }

  function ondragover(e: DragEvent) {
    const types = e.dataTransfer?.types ?? []
    const files = types.includes('Files')
    // Files from the computer are added (create), tree entries moved (organize).
    if (!e.dataTransfer || !(files ? app.can.create : types.includes(DRAG_TYPE) && app.can.organize)) return
    e.preventDefault()
    e.dataTransfer.dropEffect = files ? 'copy' : 'move'
    rootDrop = true
  }

  function ondrop(e: DragEvent) {
    e.preventDefault()
    rootDrop = false
    const entries = droppedEntries(e.dataTransfer)
    if (hasFolder(entries)) {
      void app.uploadDropped(entries, '')
      return
    }
    const files = [...(e.dataTransfer?.files ?? [])]
    if (files.length) {
      void app.uploadInto(files, '')
      return
    }
    const paths = droppedPaths(e)
    if (paths.length) void app.moveMany(paths, '')
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && app.selected.size) {
      e.preventDefault()
      app.clearSelection()
    }
  }

  const empty = $derived(app.treeLoaded && app.tree.length === 0 && !creatingAtRoot)
</script>

<div
  class="tree"
  class:drop={rootDrop}
  role="presentation"
  oncontextmenu={(e) => openMenu(e, null)}
  {onkeydown}
  {ondragover}
  ondragleave={(e) => {
    if (!(e.currentTarget as HTMLElement).contains(e.relatedTarget as Node | null)) rootDrop = false
  }}
  {ondrop}
>
  {#if app.selected.size && app.can.organize}
    <div class="selection" role="toolbar" aria-label={strings.tree.selectionLabel}>
      <span class="count">{strings.tree.selected(app.selected.size)}</span>
      <button type="button" title={strings.tree.moveTo} aria-label={strings.tree.moveTo} onclick={() => (moving = [...app.selected])}>
        <Icon name="folder-open" size={16} />
      </button>
      <button type="button" class="danger" title={strings.tree.delete} aria-label={strings.tree.delete} onclick={() => void app.removeMany([...app.selected])}>
        <Icon name="trash" size={16} />
      </button>
      <button type="button" title={strings.tree.clearSelection} aria-label={strings.tree.clearSelection} onclick={() => app.clearSelection()}>
        <Icon name="close" size={16} />
      </button>
    </div>
  {/if}
  <ul role="tree" aria-label={strings.soma.treeLabel} aria-multiselectable={app.can.organize}>
    {#if creatingAtRoot}
      <li>
        <TreeInput
          depth={0}
          kind={entryKind(creatingAtRoot.kind)}
          onsubmit={(name) => app.createEntry(entryKind(creatingAtRoot.kind), '', name)}
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
      {#if app.can.create}
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

{#if moving}
  <MoveDialog paths={moving} onclose={() => (moving = null)} />
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
  .selection {
    position: sticky;
    z-index: 1;
    top: calc(-1 * var(--space-2));
    display: flex;
    align-items: center;
    gap: 2px;
    margin: calc(-1 * var(--space-2)) calc(-1 * var(--space-2)) var(--space-2);
    padding: var(--space-1) var(--space-2) var(--space-1) var(--space-3);
    border-bottom: 1px solid var(--line);
    background: var(--bg-raised);
  }
  .count {
    flex: 1;
    color: var(--text);
    font-size: var(--text-sm);
    font-weight: 550;
  }
  .selection button {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    padding: 0;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .selection button:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .selection button.danger:hover {
    color: var(--err);
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
