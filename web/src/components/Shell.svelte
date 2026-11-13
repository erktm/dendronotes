<script lang="ts">
  import { tick, untrack } from 'svelte'
  import LinkDialog from './LinkDialog.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import { saveFile } from '../lib/download'
  import { markdownWithTitle } from '../lib/noteExport'
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
  import TrashDialog from './TrashDialog.svelte'
  import HistoryDialog from './HistoryDialog.svelte'
  import CommentsPanel from './CommentsPanel.svelte'
  import NotificationBell from './NotificationBell.svelte'
  import { comments } from '../lib/state/comments.svelte'
  import { live } from '../lib/state/live.svelte'
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
  /** Phone layout: the sidebar lies over the page. */
  let phone = $state(narrow.matches)
  $effect(() => {
    const update = () => (phone = narrow.matches)
    narrow.addEventListener('change', update)
    return () => narrow.removeEventListener('change', update)
  })

  // On phones the sidebar closes with a tap beside it, a swipe to the left
  // or Escape – not only with the rail button that opened it.
  const closeSidebar = () => (sidebarOpen = false)
  $effect(() => {
    if (!phone || !sidebarOpen) return
    const onkey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !e.defaultPrevented && !document.querySelector('dialog[open]')) closeSidebar()
    }
    window.addEventListener('keydown', onkey)
    return () => window.removeEventListener('keydown', onkey)
  })

  /** Closes the sidebar when a touch moves clearly to the left. */
  function swipeToClose(node: HTMLElement) {
    let start: { x: number; y: number } | null = null
    const down = (e: TouchEvent) => {
      const t = e.touches[0]
      start = t && e.touches.length === 1 ? { x: t.clientX, y: t.clientY } : null
    }
    const up = (e: TouchEvent) => {
      const t = e.changedTouches[0]
      if (start && t && phone && start.x - t.clientX > 60 && Math.abs(t.clientY - start.y) < 50) closeSidebar()
      start = null
    }
    node.addEventListener('touchstart', down, { passive: true })
    node.addEventListener('touchend', up, { passive: true })
    return {
      destroy() {
        node.removeEventListener('touchstart', down)
        node.removeEventListener('touchend', up)
      },
    }
  }
  /** The next route change comes from the rail and keeps the sidebar open. */
  let keepSidebar = false

  // Follow the URL: first the soma, then the note in it.
  $effect(() => {
    const soma = router.soma
    const path = router.notePath
    const view = router.view
    const link = router.link
    const loaded = app.somasLoaded
    untrack(() => {
      if (!loaded) return
      if (view === 'admin' || view === 'tutorials') {
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
      // An internal link: the server says where the entry is now.
      if (link && app.somaId === soma) {
        void app.followLink(link)
        return
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
      router.view === 'admin' ? strings.admin.title : router.view === 'tutorials' ? strings.tutorials.title : router.graph ? strings.graph.title : app.note ? displayName(app.note.path) : app.soma?.name,
    )
  })

  /** Opens the admin area, or returns to the notes from it. */
  function toggleAdmin() {
    if (router.view === 'admin') {
      router.goSoma(app.somaId || app.preferredSoma())
    } else router.goAdmin()
  }

  /** Opens the tutorials, or returns to the notes from them. */
  function toggleTutorials() {
    if (router.view === 'tutorials') router.goSoma(app.somaId || app.preferredSoma())
    else router.goTutorials()
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

  /** The graph, the admin area or the tutorials fill the main area instead of a note. */
  const mainView = $derived(router.view === 'admin' || router.view === 'tutorials' || router.graph)

  /**
   * The rail works like tabs: only one entry is marked. From the graph or the
   * admin area, files and search lead back to the notes with that panel open;
   * otherwise they show or hide the panel.
   */
  function showPanel(panel: 'files' | 'search') {
    if (mainView) {
      keepSidebar = true // on phones the route change would close it again
      if (router.view === 'admin') toggleAdmin()
      else if (router.view === 'tutorials') toggleTutorials()
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

  // --- Export: Markdown with the title on top, or the reading view printed
  let exportMenu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null)

  function openExport(e: MouseEvent) {
    const note = app.note
    if (!note) return
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    exportMenu = {
      x: r.right - 220,
      y: r.bottom + 6,
      items: [
        { label: strings.exportNote.markdown, icon: 'download', action: () => downloadMarkdown() },
        { label: strings.exportNote.print, icon: 'note', action: () => void printNote() },
      ],
    }
  }

  function downloadMarkdown() {
    const note = app.note
    if (!note) return
    const title = displayName(note.path)
    saveFile(new Blob([markdownWithTitle(title, note.content)], { type: 'text/markdown;charset=utf-8' }), `${title}.md`)
  }

  /** Prints the reading view (title, text, drawings); the browser saves it as PDF. */
  async function printNote() {
    const note = app.note
    if (!note) return
    const view = settings.view
    const title = document.title
    settings.view = 'read'
    // The PDF is named after the page title.
    document.title = displayName(note.path)
    await tick()
    // Drawings and code colors are drawn after the text.
    await new Promise((r) => setTimeout(r, 500))
    window.print()
    document.title = title
    settings.view = view
  }

  // One panel beside the note at a time: comments take the place of the
  // details (outline, synapses) while they are open.
  const detailsShown = $derived(settings.showDetails && !app.commentsOpen)
  function toggleDetails() {
    if (app.commentsOpen) {
      app.commentsOpen = false
      settings.showDetails = true
    } else {
      settings.showDetails = !settings.showDetails
    }
  }

  // The comments of the open note (also for the count in the top bar),
  // again when someone changed them or the connection came back.
  $effect(() => {
    const path = app.note?.path
    if (!path) {
      comments.clear()
      return
    }
    void live.state
    const changed = live.commentsChanged
    untrack(() => {
      if (changed.path === '' || changed.path === path || comments.path !== path) void comments.load(path)
    })
  })
  const openThreads = $derived(comments.path === app.note?.path ? comments.threads.filter((th) => !th.root.resolvedAt).length : 0)

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
    tutorials={router.view === 'tutorials'}
    ontutorials={toggleTutorials}
  />
  {#if sidebarOpen}
    {#if phone}
      <button type="button" class="scrim" tabindex="-1" aria-label={strings.nav.hideFiles} onclick={closeSidebar} use:swipeToClose></button>
    {/if}
    <div class="sidebar-slot" use:swipeToClose>
      <Sidebar />
    </div>
  {/if}
  <main class="main">
    <header class="topbar">
      <nav class="crumbs" aria-label={strings.soma.location}>
        {#if router.view === 'admin'}
          <span class="current" aria-current="page">{strings.admin.title}</span>
        {:else if router.view === 'tutorials'}
          <span class="current" aria-current="page">{strings.tutorials.title}</span>
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
            class="icon-btn counted"
            class:on={app.commentsOpen}
            aria-pressed={app.commentsOpen}
            aria-label={strings.comments.openCount(openThreads)}
            title={strings.comments.openCount(openThreads)}
            onclick={() => (app.commentsOpen = !app.commentsOpen)}
          >
            <Icon name="comment" size={18} />
            {#if openThreads}<span class="count" aria-hidden="true">{openThreads}</span>{/if}
          </button>
          {#if app.can.create && !app.note?.readOnly && settings.view === 'edit'}
            <button
              type="button"
              class="icon-btn"
              aria-label={strings.drawing.insert}
              title={strings.drawing.insert}
              onclick={() => void app.insertDrawing()}
            >
              <Icon name="drawing" size={18} />
            </button>
          {/if}
          <button
            type="button"
            class="icon-btn"
            aria-label={strings.links.open}
            title={strings.links.open}
            aria-haspopup="dialog"
            onclick={() => (app.linkDialog = app.note?.path ?? null)}
          >
            <Icon name="link" size={18} />
          </button>
          <button
            type="button"
            class="icon-btn"
            aria-label={strings.exportNote.open}
            title={strings.exportNote.open}
            aria-haspopup="menu"
            onclick={openExport}
          >
            <Icon name="download" size={18} />
          </button>
          <button
            type="button"
            class="icon-btn"
            aria-label={strings.history.open}
            title={strings.history.open}
            onclick={() => (app.historyOpen = true)}
          >
            <Icon name="history" size={18} />
          </button>
          <button
            type="button"
            class="icon-btn"
            class:on={detailsShown}
            aria-pressed={detailsShown}
            aria-label={detailsShown ? strings.view.hideDetails : strings.view.showDetails}
            title={detailsShown ? strings.view.hideDetails : strings.view.showDetails}
            onclick={toggleDetails}
          >
            <Icon name="panel" size={18} />
          </button>
        {/if}
        <NotificationBell />
        <ProfileMenu />
      </div>
    </header>
    <div class="workspace">
      <div class="content">
        {#if router.view === 'admin'}
          <AdminView />
        {:else if router.view === 'tutorials'}
          <!-- Loaded on demand: the main bundle is close to its limit. -->
          {#await import('./TutorialsView.svelte') then m}
            <m.default />
          {/await}
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
      {#if app.note && noteReady && app.commentsOpen}
        <CommentsPanel session={app.note} />
      {:else if app.note && noteReady && settings.showDetails}
        <DetailsPanel session={app.note} />
      {/if}
    </div>
  </main>
</div>

{#if app.linkDialog}
  <LinkDialog path={app.linkDialog} />
{/if}

{#if exportMenu}
  <ContextMenu x={exportMenu.x} y={exportMenu.y} items={exportMenu.items} onclose={() => (exportMenu = null)} />
{/if}

<ConfirmDialog />
<PromptDialog />
<ProfileDialog />
<ShareDialog />
<TrashDialog />
<HistoryDialog />
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

  .sidebar-slot {
    display: contents;
  }
  .scrim {
    position: fixed;
    inset: 0 0 0 var(--rail-width);
    z-index: 39;
    padding: 0;
    border: 0;
    background: var(--scrim);
    cursor: default;
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
  .counted {
    position: relative;
  }
  .count {
    position: absolute;
    top: 2px;
    right: 0;
    min-width: 15px;
    height: 15px;
    padding: 0 3px;
    border-radius: 999px;
    background: var(--warn-soft);
    color: var(--warn-text);
    font-size: 9.5px;
    font-weight: 700;
    line-height: 15px;
  }
</style>
