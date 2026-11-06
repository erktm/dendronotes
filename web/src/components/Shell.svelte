<script lang="ts">
  import { untrack } from 'svelte'
  import Rail from './Rail.svelte'
  import Sidebar from './Sidebar.svelte'
  import NoteView from './NoteView.svelte'
  import DetailsPanel from './DetailsPanel.svelte'
  import EmptyState from './EmptyState.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import QuickSwitcher from './QuickSwitcher.svelte'
  import SettingsDialog from './SettingsDialog.svelte'
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { settings } from '../lib/state/settings.svelte'
  import { router } from '../lib/router.svelte'
  import { displayName } from '../lib/paths'
  import { strings } from '../lib/strings'

  const narrow = window.matchMedia('(max-width: 760px)')
  let sidebarOpen = $state(!narrow.matches)

  // Follow the URL.
  $effect(() => {
    const path = router.notePath
    untrack(() => app.openPath(path))
    if (narrow.matches) sidebarOpen = false
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
  /** An editable note is open (not loading, missing or an attachment). */
  const noteReady = $derived(app.note?.status === 'ready')

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
    if (!(e.metaKey || e.ctrlKey) || e.altKey) return
    const key = e.key.toLowerCase()
    if (key === 's') {
      e.preventDefault()
      void app.note?.save()
    } else if (key === 'f' && e.shiftKey) {
      e.preventDefault()
      sidebarOpen = true
      app.openSearch()
    } else if (key === 'o') {
      e.preventDefault()
      app.switcherOpen = true
    } else if (key === 'e' && noteReady) {
      e.preventDefault()
      settings.toggleView()
    } else if (key === ',') {
      e.preventDefault()
      app.settingsOpen = true
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

<div class="shell" class:collapsed={!sidebarOpen}>
  <Rail
    view={sidebarOpen ? app.sidebarView : null}
    onfiles={() => {
      if (sidebarOpen && app.sidebarView === 'files') sidebarOpen = false
      else ((sidebarOpen = true), (app.sidebarView = 'files'))
    }}
    onsearch={() => {
      if (sidebarOpen && app.sidebarView === 'search') sidebarOpen = false
      else ((sidebarOpen = true), app.openSearch())
    }}
    onopensettings={() => (app.settingsOpen = true)}
  />
  {#if sidebarOpen}
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
      <div class="tools">
        {#if save}
          <span class="save {save.kind}" role="status">{save.label}</span>
        {/if}
        {#if noteReady}
          <div class="segmented" role="group" aria-label={strings.view.label}>
            <button type="button" aria-pressed={settings.view === 'edit'} title={strings.view.editHint} onclick={() => (settings.view = 'edit')}>
              {strings.view.edit}
            </button>
            <button type="button" aria-pressed={settings.view === 'read'} title={strings.view.readHint} onclick={() => (settings.view = 'read')}>
              {strings.view.read}
            </button>
          </div>
          <button
            type="button"
            class="icon-btn"
            class:on={settings.showDetails}
            aria-pressed={settings.showDetails}
            aria-label={settings.showDetails ? strings.view.hideDetails : strings.view.showDetails}
            title={settings.showDetails ? strings.view.hideDetails : strings.view.showDetails}
            onclick={() => (settings.showDetails = !settings.showDetails)}
          >
            <Icon name="panel" size={18} />
          </button>
        {/if}
      </div>
    </header>
    <div class="workspace">
      <div class="content">
        {#if app.note}
          {#key app.note}
            <NoteView session={app.note} />
          {/key}
        {:else}
          <EmptyState />
        {/if}
      </div>
      {#if app.note && noteReady && settings.showDetails && app.note.conflict === null}
        <DetailsPanel session={app.note} />
      {/if}
    </div>
  </main>
</div>

<ConfirmDialog />
<QuickSwitcher />
<SettingsDialog />

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
    padding: 0 var(--space-3) 0 var(--space-6);
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
  .tools {
    display: flex;
    flex: none;
    align-items: center;
    gap: 10px;
  }
  .save {
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
    color: var(--warn-text);
    font-weight: 600;
  }
  .save.err {
    color: var(--err);
    font-weight: 600;
  }
  .segmented {
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg-sunken);
  }
  .segmented button {
    height: 26px;
    padding: 0 12px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text-muted);
    font-size: 12.5px;
    font-weight: 550;
    cursor: pointer;
  }
  .segmented button[aria-pressed='true'] {
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow);
  }
  .icon-btn {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .icon-btn.on {
    color: var(--accent);
  }
  .workspace {
    display: flex;
    flex: 1;
    min-height: 0;
  }
  .content {
    flex: 1;
    min-width: 0;
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
    .save {
      display: none;
    }
  }
</style>
