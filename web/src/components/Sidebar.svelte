<script lang="ts">
  import { onMount } from 'svelte'
  import FileTree from './FileTree.svelte'
  import SearchPanel from './SearchPanel.svelte'
  import SomaSwitcher from './SomaSwitcher.svelte'
  import Icon from './Icon.svelte'
  import StatusChip from './StatusChip.svelte'
  import { app } from '../lib/state/app.svelte'
  import { live } from '../lib/state/live.svelte'
  import { checkReady, type ServerState } from '../lib/api/health'
  import { dirname } from '../lib/paths'
  import { strings } from '../lib/strings'
  import Wordmark from './Wordmark.svelte'
  import { UPLOAD_EXTENSIONS } from '../lib/files'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import { SORT_MODES } from '../lib/tree'

  let serverState = $state<ServerState>('checking')

  onMount(() => {
    const ctrl = new AbortController()
    const poll = async () => {
      serverState = await checkReady(ctrl.signal)
    }
    void poll()
    const timer = window.setInterval(poll, 15_000)
    return () => {
      ctrl.abort()
      window.clearInterval(timer)
    }
  })

  // The live stream knows first when the connection drops; /readyz tells
  // whether the server can work (soma reachable, index built).
  const shownState = $derived.by((): ServerState => {
    if (live.state === 'live') return serverState === 'unavailable' ? 'unavailable' : 'ready'
    if (live.state === 'connecting') return 'checking'
    return serverState === 'ready' ? 'checking' : serverState
  })

  let picker: HTMLInputElement | undefined = $state()

  // New entries go next to the open note.
  const targetFolder = () => (app.note ? dirname(app.note.path) : '')

  // The arrow next to "New note" offers the other kinds.
  let newMenu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null)

  // How this person sorts the tree; sorting by hand shows the shared order.
  function openSortMenu(e: MouseEvent) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    newMenu = {
      x: r.left,
      y: r.bottom + 4,
      items: SORT_MODES.map((m) => ({
        label: strings.tree.sortModes[m]!,
        icon: 'check' as const,
        checked: app.sortMode === m,
        action: () => app.setSort(m),
      })),
    }
  }

  function openNewMenu(e: MouseEvent) {
    const r = (e.currentTarget as HTMLElement).closest('.split')!.getBoundingClientRect()
    newMenu = {
      x: r.left,
      y: r.bottom + 4,
      items: [
        { label: strings.tree.newNote, icon: 'note', action: () => app.startCreate('new-note', targetFolder()) },
        { label: strings.tree.newDrawing, icon: 'drawing', action: () => app.startCreate('new-drawing', targetFolder()) },
      ],
    }
  }
</script>

<aside class="sidebar" aria-label={app.sidebarView === 'search' ? strings.search.title : strings.nav.files}>
  {#if app.sidebarView === 'search'}
    <SearchPanel />
  {:else}
  <header class="head">
    <Wordmark />
    <div class="actions">
      {#if app.can.create}
      <div class="split">
        <button type="button" aria-label={strings.tree.newNote} title={strings.tree.newNote} onclick={() => app.startCreate('new-note', targetFolder())}>
          <Icon name="plus" size={18} />
        </button>
        <button
          type="button"
          class="more"
          aria-label={strings.tree.newMenu}
          title={strings.tree.newMenu}
          aria-haspopup="menu"
          aria-expanded={newMenu !== null}
          onclick={openNewMenu}
        >
          <Icon name="chevron-down" size={13} />
        </button>
      </div>
      <button
        type="button"
        aria-label={strings.tree.newFolder}
        title={strings.tree.newFolder}
        onclick={() => app.startCreate('new-folder', targetFolder())}
      >
        <Icon name="folder-plus" size={18} />
      </button>
      <button type="button" aria-label={strings.files.upload} title={strings.files.uploadHint} onclick={() => picker?.click()}>
        <Icon name="upload" size={18} />
      </button>
      <input
        bind:this={picker}
        class="picker"
        type="file"
        multiple
        accept={UPLOAD_EXTENSIONS.join(',')}
        tabindex="-1"
        aria-hidden="true"
        onchange={() => {
          const files = [...(picker?.files ?? [])]
          if (picker) picker.value = ''
          if (files.length) void app.uploadInto(files, targetFolder())
        }}
      />
      {/if}
      <button type="button" aria-label={strings.tree.sortBy} title={strings.tree.sortBy} aria-haspopup="menu" onclick={openSortMenu}>
        <Icon name="sort" size={18} />
      </button>
    </div>
  </header>

  <SomaSwitcher />
  <FileTree />
  {/if}

  {#if newMenu}
    <ContextMenu x={newMenu.x} y={newMenu.y} items={newMenu.items} onclose={() => (newMenu = null)} />
  {/if}

  <footer class="foot">
    <StatusChip state={shownState} />
    <button type="button" class="about" title={strings.about.title} onclick={() => (app.aboutOpen = true)}>{strings.about.open}</button>
    {#if app.sidebarView !== 'search' && app.can.organize}
      <button type="button" class="trash" aria-label={strings.trash.open} title={strings.trash.open} onclick={() => (app.trashOpen = true)}>
        <Icon name="trash" size={16} />
      </button>
    {/if}
  </footer>
</aside>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    background: var(--bg-raised);
    border-right: 1px solid var(--line);
  }
  .head {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: space-between;
    height: 48px;
    padding: 0 var(--space-2) 0 var(--space-4);
    border-bottom: 1px solid var(--line);
  }
  .actions {
    display: flex;
    gap: 2px;
  }
  .picker {
    display: none;
  }
  button {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  button:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .trash {
    margin-left: auto;
  }
  /* A quiet text link next to the connection state. */
  button.about {
    width: auto;
    padding: 0 var(--space-2);
    color: var(--text-faint);
    font-size: var(--text-xs);
  }
  button.about:hover {
    color: var(--text);
  }
  .split {
    display: flex;
    border-radius: var(--radius-sm);
  }
  .split:hover {
    background: var(--bg-hover);
  }
  /* One control: the arrow sits right next to the plus. */
  .split button {
    width: 22px;
    padding: 0;
    place-items: center end;
  }
  .split button:hover {
    background: transparent;
  }
  .split .more {
    width: 14px;
    margin-left: -2px;
    place-items: center start;
  }
  .foot {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-2) var(--space-2) var(--space-3);
    border-top: 1px solid var(--line);
  }
</style>
