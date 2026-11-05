<script lang="ts">
  import { onMount } from 'svelte'
  import FileTree from './FileTree.svelte'
  import Icon from './Icon.svelte'
  import StatusChip from './StatusChip.svelte'
  import { app } from '../lib/state/app.svelte'
  import { checkReady, type ServerState } from '../lib/api/health'
  import { dirname } from '../lib/paths'

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

  // New entries go next to the open note.
  const targetFolder = () => (app.note ? dirname(app.note.path) : '')
</script>

<aside class="sidebar" aria-label="Files">
  <header class="head">
    <span class="wordmark">leaf<span>notes</span></span>
    <div class="actions">
      <button type="button" aria-label="New note" title="New note" onclick={() => app.startCreate('new-note', targetFolder())}>
        <Icon name="plus" size={18} />
      </button>
      <button
        type="button"
        aria-label="New folder"
        title="New folder"
        onclick={() => app.startCreate('new-folder', targetFolder())}
      >
        <Icon name="folder-plus" size={18} />
      </button>
    </div>
  </header>

  <FileTree />

  <footer class="foot">
    <StatusChip state={serverState} />
    <span class="user" title="Signed in as {app.user}">{app.user}</span>
    <button type="button" aria-label="Sign out" title="Sign out" onclick={() => void app.logout()}>
      <Icon name="logout" size={17} />
    </button>
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
  .wordmark {
    font-size: var(--text-lg);
    font-weight: 650;
    letter-spacing: -0.02em;
    color: var(--accent);
  }
  .wordmark span {
    color: var(--text);
    font-weight: 500;
  }
  .actions {
    display: flex;
    gap: 2px;
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
  .user {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    color: var(--text-muted);
    font-size: var(--text-sm);
    text-align: right;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
</style>
