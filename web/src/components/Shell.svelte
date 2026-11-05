<script lang="ts">
  import { untrack } from 'svelte'
  import Rail from './Rail.svelte'
  import Sidebar from './Sidebar.svelte'
  import NoteView from './NoteView.svelte'
  import EmptyState from './EmptyState.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import { app } from '../lib/state/app.svelte'
  import { router } from '../lib/router.svelte'
  import { displayName } from '../lib/paths'
  import { strings } from '../lib/strings'

  const narrow = window.matchMedia('(max-width: 760px)')
  let filesOpen = $state(!narrow.matches)

  // Follow the URL.
  $effect(() => {
    const path = router.notePath
    untrack(() => app.openPath(path))
    if (narrow.matches) filesOpen = false
  })

  // Remember which folders are open.
  $effect(() => {
    void [...app.expanded]
    untrack(() => app.persistExpanded())
  })

  $effect(() => {
    document.title = strings.app.title(app.note ? displayName(app.note.path) : undefined)
  })

  const crumbs = $derived(app.note ? app.note.path.split('/') : [])

  const save = $derived.by(() => {
    const n = app.note
    if (!n || n.status !== 'ready') return null
    if (n.readOnly) return { kind: 'muted', label: strings.saveState.readOnly }
    if (n.conflict !== null) return { kind: 'warn', label: strings.saveState.conflict }
    if (n.saving) return { kind: 'muted', label: strings.saveState.saving }
    if (n.failed) return { kind: 'err', label: strings.saveState.failed }
    if (n.dirty) return { kind: 'muted', label: strings.saveState.edited }
    return { kind: 'ok', label: strings.saveState.saved }
  })

  function onkeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
      e.preventDefault()
      void app.note?.save()
    }
  }

  function onbeforeunload(e: BeforeUnloadEvent) {
    const n = app.note
    if (n && (n.dirty || n.conflict !== null)) {
      void n.flush()
      e.preventDefault()
    }
  }

  function onvisibilitychange() {
    if (document.visibilityState === 'hidden') void app.note?.flush()
  }
</script>

<svelte:window {onkeydown} {onbeforeunload} />
<svelte:document {onvisibilitychange} />

<div class="shell" class:collapsed={!filesOpen}>
  <Rail {filesOpen} ontogglefiles={() => (filesOpen = !filesOpen)} />
  {#if filesOpen}
    <Sidebar />
  {/if}
  <main class="main">
    <header class="topbar">
      <nav class="crumbs" aria-label={strings.soma.location}>
        {#if app.note}
          {#each crumbs as part, i (i)}
            {#if i > 0}<span class="sep" aria-hidden="true">/</span>{/if}
            {#if i === crumbs.length - 1}
              <span class="current" aria-current="page">{displayName(part)}</span>
            {:else}
              <span>{part}</span>
            {/if}
          {/each}
        {:else}
          <span>{strings.soma.label}</span>
        {/if}
      </nav>
      {#if save}
        <span class="save {save.kind}" role="status">{save.label}</span>
      {/if}
    </header>
    <div class="content">
      {#if app.note}
        {#key app.note}
          <NoteView session={app.note} />
        {/key}
      {:else}
        <EmptyState />
      {/if}
    </div>
  </main>
</div>

<ConfirmDialog />

<style>
  .shell {
    display: grid;
    grid-template-columns: var(--rail-width) var(--sidebar-width) minmax(0, 1fr);
    height: 100%;
  }
  .shell.collapsed {
    grid-template-columns: var(--rail-width) minmax(0, 1fr);
  }
  .main {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }
  .topbar {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    height: 48px;
    padding: 0 var(--space-4) 0 var(--space-6);
    border-bottom: 1px solid var(--line);
  }
  .crumbs {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    overflow: hidden;
    color: var(--text-muted);
    font-size: var(--text-sm);
    white-space: nowrap;
  }
  .crumbs .sep {
    color: var(--text-faint);
  }
  .crumbs .current {
    overflow: hidden;
    color: var(--text);
    font-weight: 550;
    text-overflow: ellipsis;
  }
  .save {
    flex: none;
    font-size: var(--text-xs);
    color: var(--text-faint);
  }
  .save.ok::before {
    content: '';
    display: inline-block;
    width: 6px;
    height: 6px;
    margin-right: 6px;
    border-radius: 50%;
    background: var(--ok);
    vertical-align: 1px;
  }
  .save.warn {
    color: var(--warn);
    font-weight: 600;
  }
  .save.err {
    color: var(--err);
    font-weight: 600;
  }
  .content {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }

  /* Phones: the file list slides over the note instead of beside it. */
  @media (max-width: 760px) {
    .shell,
    .shell.collapsed {
      grid-template-columns: var(--rail-width) minmax(0, 1fr);
    }
    .shell :global(.sidebar) {
      position: fixed;
      inset: 0 auto 0 var(--rail-width);
      z-index: 40;
      width: min(85vw, var(--sidebar-width));
      box-shadow: var(--shadow), 0 0 40px rgb(0 0 0 / 0.2);
    }
    .topbar {
      padding-left: var(--space-4);
    }
  }
</style>
