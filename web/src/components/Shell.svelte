<script lang="ts">
  import { untrack } from 'svelte'
  import Rail from './Rail.svelte'
  import Sidebar from './Sidebar.svelte'
  import NoteView from './NoteView.svelte'
  import DetailsPanel from './DetailsPanel.svelte'
  import GraphView from './GraphView.svelte'
  import AdminView from './AdminView.svelte'
  import PromptDialog from './PromptDialog.svelte'
  import ProfileDialog from './ProfileDialog.svelte'
  import ProfileMenu from './ProfileMenu.svelte'
  import ShareDialog from './ShareDialog.svelte'
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
  /** The next route change comes from the rail and keeps the sidebar open. */
  let keepSidebar = false

  // Follow the URL: first the soma, then the note in it.
  $effect(() => {
    const soma = router.soma
    const path = router.notePath
    const view = router.view
    const loaded = app.somasLoaded
    untrack(() => {
      if (!loaded) return
      if (view === 'admin') {
        // Keep the sidebar useful in the admin area: open the last soma behind it.
        if (!app.somaId && app.preferredSoma()) app.useSoma(app.preferredSoma())
      } else {
        if (soma && app.somas.some((s) => s.id === soma)) {
          app.useSoma(soma)
        } else if (soma) {
          app.toast(strings.somas.notFound)
          router.goSoma(app.somas[0]?.id ?? '', true)
          return
        } else if (app.somas[0]) {
          router.goSoma(app.somas[0].id, true)
          return
        }
      }
      app.openPath(app.somaId && soma === app.somaId && view === 'soma' ? path : null)
    })
    if (keepSidebar) keepSidebar = false
    else if (narrow.matches) sidebarOpen = false
  })

  // Remember which folders are open.
  $effect(() => {
    void [...app.expanded]
    untrack(() => app.persistExpanded())
  })

  $effect(() => {
    document.title = strings.app.title(
      router.view === 'admin' ? strings.admin.title : router.graph ? strings.graph.title : app.note ? displayName(app.note.path) : app.soma?.name,
    )
  })

  /** Opens the admin area, or returns to the notes from it. */
  function toggleAdmin() {
    if (router.view === 'admin') {
      router.goSoma(app.somaId || app.preferredSoma())
    } else router.goAdmin()
  }

  async function createSoma() {
    const name = await app.prompt({
      title: strings.somas.createTitle,
      text: strings.somas.createText,
      label: strings.somas.nameLabel,
      value: '',
      placeholder: strings.somas.namePlaceholder,
      confirmLabel: strings.somas.createButton,
    })
    if (name) await app.createSoma(name)
  }

  /** Opens the graph, or returns to the last note from it. */
  function toggleGraph() {
    if (router.graph) void app.navigate(app.lastNotePath)
    else router.goGraph()
  }

  /** The graph or the admin area fills the main area instead of a note. */
  const mainView = $derived(router.view === 'admin' || router.graph)

  /**
   * The rail works like tabs: only one entry is marked. From the graph or the
   * admin area, files and search lead back to the notes with that panel open;
   * otherwise they show or hide the panel.
   */
  function showPanel(panel: 'files' | 'search') {
    if (mainView) {
      keepSidebar = true // on phones the route change would close it again
      if (router.view === 'admin') toggleAdmin()
      else toggleGraph()
    } else if (sidebarOpen && app.sidebarView === panel) {
      sidebarOpen = false
      return
    }
    sidebarOpen = true
    if (panel === 'search') app.openSearch()
    else app.sidebarView = 'files'
  }

  const crumbs = $derived(app.note ? app.note.path.split('/') : [])
  /** An editable note is open (not loading, missing or an attachment). */
  const noteReady = $derived(app.note?.status === 'ready')

  const save = $derived.by(() => {
    const n = app.note
    if (!n || n.status !== 'ready') return null
    if (n.readOnly) return { kind: 'muted', label: strings.saveState.readOnly }
    if (n.failed) return { kind: 'err', label: strings.saveState.failed }
    if (n.offline && n.unsaved) return { kind: 'warn', label: strings.saveState.offline }
    if (n.unsaved) return { kind: 'muted', label: strings.saveState.saving }
    return { kind: 'ok', label: strings.saveState.saved }
  })

  function onkeydown(e: KeyboardEvent) {
    if (!(e.metaKey || e.ctrlKey) || e.altKey) return
    const key = e.key.toLowerCase()
    if (key === 's') {
      e.preventDefault()
      void app.note?.push(true)
    } else if (key === 'g' && e.shiftKey) {
      e.preventDefault()
      toggleGraph()
    } else if (key === 'f' && e.shiftKey) {
      e.preventDefault()
      sidebarOpen = true
      app.openSearch()
    } else if (key === 'o') {
      e.preventDefault()
      app.switcherOpen = true
    } else if (key === 'e' && noteReady && !app.note?.readOnly) {
      e.preventDefault()
      settings.toggleView()
    } else if (key === ',') {
      e.preventDefault()
      app.settingsOpen = true
    }
  }

  function onbeforeunload(e: BeforeUnloadEvent) {
    // Changes the server already has are saved by it; only unsent ones are at risk.
    const n = app.note
    if (n && n.pending > 0) {
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
    view={sidebarOpen && !mainView ? app.sidebarView : null}
    onfiles={() => showPanel('files')}
    onsearch={() => showPanel('search')}
    onopensettings={() => (app.settingsOpen = true)}
    graph={router.graph}
    ongraph={toggleGraph}
    admin={router.view === 'admin'}
    onadmin={toggleAdmin}
  />
  {#if sidebarOpen}
    <Sidebar />
  {/if}
  <main class="main">
    <header class="topbar">
      <nav class="crumbs" aria-label={strings.soma.location}>
        {#if router.view === 'admin'}
          <span class="current" aria-current="page">{strings.admin.title}</span>
        {:else if router.graph}
          {#if app.soma}<span>{app.soma.name}</span><span class="sep" aria-hidden="true">/</span>{/if}
          <span class="current" aria-current="page">{strings.graph.title}</span>
        {:else if app.note}
          {#if app.soma}<span class="soma-crumb">{app.soma.name}</span><span class="sep soma-crumb" aria-hidden="true">/</span>{/if}
          {#each crumbs as part, i (i)}
            {#if i > 0}<span class="sep" aria-hidden="true">/</span>{/if}
            {#if i === crumbs.length - 1}
              <span class="current" aria-current="page">{displayName(part)}</span>
            {:else}
              <span>{part}</span>
            {/if}
          {/each}
        {:else}
          <span class="current">{app.soma?.name ?? strings.soma.label}</span>
        {/if}
      </nav>
      <div class="tools">
        {#if save}
          <span class="save {save.kind}" role="status">{save.label}</span>
        {/if}
        {#if noteReady}
          <!-- Notes you can't change only have the reading view (lib/view.ts). -->
          {#if !app.note?.readOnly}
            <div class="segmented" role="group" aria-label={strings.view.label}>
              <button type="button" aria-pressed={settings.view === 'edit'} title={strings.view.editHint} onclick={() => (settings.view = 'edit')}>
                {strings.view.edit}
              </button>
              <button type="button" aria-pressed={settings.view === 'read'} title={strings.view.readHint} onclick={() => (settings.view = 'read')}>
                {strings.view.read}
              </button>
            </div>
          {/if}
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
        <ProfileMenu />
      </div>
    </header>
    <div class="workspace">
      <div class="content">
        {#if router.view === 'admin'}
          <AdminView />
        {:else if router.graph}
          {#key app.somaId}
            <GraphView center={app.lastNotePath} />
          {/key}
        {:else if app.note}
          {#key app.note}
            <NoteView session={app.note} />
          {/key}
        {:else if app.somasLoaded && app.somas.length === 0}
          <EmptyState title={strings.somas.none} text={strings.somas.noneText}>
            <button type="button" class="button" onclick={createSoma}>{strings.somas.create}</button>
          </EmptyState>
        {:else}
          <EmptyState />
        {/if}
      </div>
      {#if app.note && noteReady && settings.showDetails}
        <DetailsPanel session={app.note} />
      {/if}
    </div>
  </main>
</div>

<ConfirmDialog />
<PromptDialog />
<ProfileDialog />
<ShareDialog />
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
