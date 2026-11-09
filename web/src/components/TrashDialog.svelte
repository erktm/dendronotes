<script lang="ts">
  import { closeOnBackdrop } from '../lib/actions'
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { live } from '../lib/state/live.svelte'
  import * as api from '../lib/api/soma'
  import type { Note, TrashItem } from '../lib/api/soma'
  import { errorMessage } from '../lib/errors'
  import { basename, dirname, displayName, isNote } from '../lib/paths'
  import { formatSize } from '../lib/files'
  import { ago } from '../lib/time'
  import { strings } from '../lib/strings'

  const t = strings.trash

  let dialog: HTMLDialogElement | undefined = $state()
  let items = $state.raw<TrashItem[] | null>(null)
  let preview = $state.raw<{ item: TrashItem; note: Note } | null>(null)
  let busy = $state(false)

  $effect(() => {
    if (!dialog) return
    if (app.trashOpen && !dialog.open) {
      items = null
      preview = null
      dialog.showModal()
    } else if (!app.trashOpen && dialog.open) dialog.close()
  })

  // Reload while open: something was deleted, restored or purged.
  $effect(() => {
    void live.version
    void live.trashVersion
    if (app.trashOpen) void load()
  })

  async function load() {
    try {
      items = await api.trashItems()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  const nameOf = (it: TrashItem) => (it.type === 'file' && isNote(it.original) ? displayName(it.original) : basename(it.original))
  const who = (it: TrashItem) => (it.deletedBy ? it.deletedBy.displayName || it.deletedBy.username : '')

  function details(it: TrashItem): string {
    const when = ago(it.deletedAt)
    const parts = [t.from(dirname(it.original)), who(it) ? t.deletedBy(who(it), when) : t.deleted(when)]
    parts.push(it.type === 'dir' ? t.files(it.files ?? 0) : formatSize(it.size))
    return parts.join(' · ')
  }

  async function act(run: () => Promise<void>) {
    busy = true
    try {
      await run()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
      await load()
    }
  }

  const restore = (it: TrashItem) =>
    act(async () => {
      await api.restoreTrash(it.path)
      if (preview?.item.path === it.path) preview = null
      app.toast(t.restored(nameOf(it)), 'info')
    })

  async function purge(it: TrashItem) {
    const ok = await app.ask({ title: t.purgeTitle(nameOf(it)), message: t.purgeText, confirmLabel: t.purge, danger: true })
    if (!ok) return
    await act(async () => {
      await api.purgeTrash(it.path)
      if (preview?.item.path === it.path) preview = null
      app.toast(t.purged(nameOf(it)), 'info')
    })
  }

  async function emptyAll() {
    const n = items?.length ?? 0
    const ok = await app.ask({ title: t.emptyTitle, message: t.emptyText(n), confirmLabel: t.emptyAll, danger: true })
    if (!ok) return
    await act(async () => {
      await api.emptyTrash()
      preview = null
      app.toast(t.emptied, 'info')
    })
  }

  async function show(it: TrashItem) {
    try {
      preview = { item: it, note: await api.readTrashNote(it.path) }
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }
</script>

<dialog bind:this={dialog} use:closeOnBackdrop aria-labelledby="trash-title" onclose={() => (app.trashOpen = false)}>
  <header>
    {#if preview}
      <button type="button" class="icon-btn" aria-label={t.backToList} title={t.backToList} onclick={() => (preview = null)}>
        <Icon name="chevron" size={18} />
      </button>
    {/if}
    <h2 id="trash-title">{preview ? nameOf(preview.item) : t.title}</h2>
    <button type="button" class="icon-btn" aria-label={t.close} onclick={() => (app.trashOpen = false)}>
      <Icon name="close" size={18} />
    </button>
  </header>

  {#if preview}
    {@const it = preview.item}
    <p class="sub">{details(it)}</p>
    <pre class="preview" aria-label={t.previewOf(nameOf(it))}>{preview.note.content}</pre>
    <div class="actions">
      {#if app.can.manage}
        <button type="button" class="btn danger" disabled={busy} onclick={() => void purge(it)}>{t.purge}</button>
      {/if}
      <button type="button" class="btn primary" disabled={busy} onclick={() => void restore(it)}>{t.restore}</button>
    </div>
  {:else}
    <div class="intro">
      <p>{t.intro}</p>
      {#if app.can.manage && items?.length}
        <button type="button" class="btn danger" disabled={busy} onclick={() => void emptyAll()}>{t.emptyAll}</button>
      {/if}
    </div>
    {#if items === null}
      <p class="none">{t.loading}</p>
    {:else if items.length === 0}
      <p class="none">{t.empty}</p>
    {:else}
      <ul class="items">
        {#each items as it (it.path)}
          <li>
            <span class="kind" aria-hidden="true"><Icon name={it.type === 'dir' ? 'folder' : isNote(it.original) ? 'note' : 'file'} size={17} /></span>
            <span class="what">
              <strong>{nameOf(it)}</strong>
              <span class="sub">{details(it)}</span>
            </span>
            {#if it.type === 'file' && isNote(it.path)}
              <button type="button" class="btn" disabled={busy} aria-label={t.previewOf(nameOf(it))} onclick={() => void show(it)}>{t.preview}</button>
            {/if}
            <button type="button" class="btn" disabled={busy} aria-label={t.restoreOf(nameOf(it))} onclick={() => void restore(it)}>
              <Icon name="restore" size={15} />
              {t.restore}
            </button>
            {#if app.can.manage}
              <button
                type="button"
                class="icon-btn"
                disabled={busy}
                aria-label={t.purgeOf(nameOf(it))}
                title={t.purgeOf(nameOf(it))}
                onclick={() => void purge(it)}
              >
                <Icon name="trash" size={16} />
              </button>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</dialog>

<style>
  dialog {
    width: min(92vw, 640px);
    max-height: 86vh;
    padding: 0;
    overflow: auto;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow), 0 24px 64px rgb(0 0 0 / 0.2);
  }
  dialog::backdrop {
    background: var(--scrim);
  }
  header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 14px 14px 14px 20px;
    border-bottom: 1px solid var(--line);
  }
  header :global(svg) {
    flex: none;
  }
  header .icon-btn:first-child:not(:last-child) {
    margin-left: -10px;
    transform: rotate(180deg);
  }
  h2 {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    font-size: var(--text-lg);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .intro {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: 14px 20px;
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .intro p {
    flex: 1;
  }
  .none {
    padding: 28px 20px 36px;
    color: var(--text-faint);
    text-align: center;
  }
  .items {
    margin: 0;
    padding: 0 12px 12px;
    list-style: none;
  }
  .items li {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 54px;
    padding: 6px 8px;
    border-radius: var(--radius-sm);
  }
  .items li + li {
    border-top: 1px solid var(--line);
  }
  .kind {
    display: grid;
    flex: none;
    place-items: center;
    width: 30px;
    height: 30px;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    color: var(--text-muted);
  }
  .what {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }
  .what strong {
    overflow: hidden;
    font-size: var(--text-sm);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sub {
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  p.sub {
    padding: 12px 20px 0;
  }
  .preview {
    max-height: 50vh;
    margin: 12px 20px;
    padding: 12px 14px;
    overflow: auto;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    font-family: var(--font-mono);
    font-size: 12.5px;
    line-height: 1.55;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: 0 20px 18px;
  }
  .btn {
    display: inline-flex;
    flex: none;
    align-items: center;
    gap: 6px;
    height: 30px;
    padding: 0 10px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    color: var(--text);
    font-size: var(--text-sm);
    font-weight: 550;
    cursor: pointer;
  }
  .btn:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .btn.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--on-accent);
  }
  .btn.danger {
    color: var(--err);
  }
  .icon-btn {
    display: grid;
    flex: none;
    place-items: center;
    width: 32px;
    height: 32px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .icon-btn:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text);
  }
  @media (max-width: 560px) {
    .items li {
      flex-wrap: wrap;
    }
    .what {
      flex-basis: calc(100% - 40px);
    }
  }
</style>
