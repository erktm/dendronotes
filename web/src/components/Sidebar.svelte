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
</script>

<aside class="sidebar" aria-label={app.sidebarView === 'search' ? strings.search.title : strings.nav.files}>
  {#if app.sidebarView === 'search'}
    <SearchPanel />
  {:else}
  <header class="head">
    <Wordmark />
    <div class="actions">
      {#if app.canEdit}
      <button type="button" aria-label={strings.tree.newNote} title={strings.tree.newNote} onclick={() => app.startCreate('new-note', targetFolder())}>
        <Icon name="plus" size={18} />
      </button>
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
    </div>
  </header>

  <SomaSwitcher />
  <FileTree />
  {/if}

  <footer class="foot">
    <StatusChip state={shownState} />
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
  .foot {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-2) var(--space-2) var(--space-3);
    border-top: 1px solid var(--line);
  }
</style>
