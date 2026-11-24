<script lang="ts">
  // Picks where a copy of a file or folder goes: a soma the account may
  // create entries in (this one or another) and a folder of it.
  import Icon from './Icon.svelte'
  import { closeOnBackdrop, cssVars } from '../lib/actions'
  import * as api from '../lib/api/soma'
  import { app } from '../lib/state/app.svelte'
  import { errorMessage } from '../lib/errors'
  import { basename, displayName } from '../lib/paths'
  import { strings } from '../lib/strings'

  let { path, onclose }: { path: string; onclose: () => void } = $props()

  const t = strings.copy
  let dialog: HTMLDialogElement | undefined = $state()
  // The open soma first when a copy may go there, else the first that takes one.
  let somaId = $state(app.copyTargets.find((s) => s.id === app.somaId)?.id ?? app.copyTargets[0]?.id ?? '')
  let target = $state('')
  let busy = $state(false)
  /** The folders of the chosen soma; null while they load. */
  let folders = $state<string[] | null>(null)

  const sortFolders = (entries: readonly api.Entry[]) =>
    entries
      .filter((e) => e.type === 'dir')
      .map((e) => e.path)
      .sort((a, b) => a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' }))

  $effect(() => {
    if (dialog && !dialog.open) dialog.showModal()
  })

  // Load the folders whenever another soma is chosen (the open one is known).
  $effect(() => {
    const id = somaId
    target = ''
    if (!id) return
    if (id === app.somaId) {
      folders = sortFolders(app.entries)
      return
    }
    folders = null
    let stale = false
    api
      .treeOf(id)
      .then((r) => !stale && (folders = sortFolders(r.entries)))
      .catch((err) => {
        if (stale) return
        app.toast(errorMessage(err))
        folders = []
      })
    return () => (stale = true)
  })

  async function copy() {
    if (!somaId || busy) return
    busy = true
    const ok = await app.copyTo(path, somaId, target)
    busy = false
    if (ok) dialog?.close()
  }
</script>

<dialog bind:this={dialog} use:closeOnBackdrop aria-labelledby="copy-title" {onclose}>
  <h2 id="copy-title">{t.title(displayName(path))}</h2>
  <label class="soma">
    <span>{t.soma}</span>
    <select bind:value={somaId}>
      {#each app.copyTargets as s (s.id)}
        <option value={s.id}>{s.name}</option>
      {/each}
    </select>
  </label>
  <div class="list" role="listbox" aria-label={t.folder}>
    {#if folders === null}
      <p class="loading">{t.loading}</p>
    {:else}
      {#each ['', ...folders] as f (f)}
        <button
          type="button"
          role="option"
          class="folder"
          aria-selected={target === f}
          use:cssVars={{ '--depth': f ? f.split('/').length - 1 : 0 }}
          onclick={() => (target = f)}
          ondblclick={() => {
            target = f
            void copy()
          }}
        >
          <Icon name={f ? 'folder' : 'folder-open'} size={16} />
          <span>{f ? basename(f) : strings.tree.topLevel}</span>
        </button>
      {/each}
    {/if}
  </div>
  <p class="hint">{t.hint}</p>
  <div class="actions">
    <button type="button" class="secondary" onclick={() => dialog?.close()}>{strings.dialogs.cancel}</button>
    <button type="button" class="confirm" disabled={!somaId || folders === null || busy} onclick={copy}>{t.copy}</button>
  </div>
</dialog>

<style>
  dialog {
    width: min(92vw, 440px);
    padding: var(--space-6);
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow), 0 24px 64px rgb(0 0 0 / 0.2);
  }
  dialog::backdrop {
    background: var(--scrim);
  }
  h2 {
    margin-bottom: var(--space-3);
    overflow: hidden;
    font-size: var(--text-lg);
    font-weight: 600;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .soma {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .soma select {
    flex: 1;
    height: 32px;
    padding: 0 var(--space-2);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
  }
  .list {
    max-height: min(50vh, 420px);
    overflow: auto;
    padding: 4px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
  }
  .loading {
    padding: var(--space-2);
    color: var(--text-faint);
    font-size: var(--text-sm);
  }
  .folder {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    height: 30px;
    padding: 0 var(--space-2) 0 calc(var(--space-2) + var(--depth) * 14px);
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text-muted);
    font-size: var(--text-sm);
    text-align: left;
    cursor: pointer;
  }
  .folder span {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .folder:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .folder[aria-selected='true'] {
    background: var(--accent-soft);
    color: var(--text);
    font-weight: 550;
  }
  .hint {
    margin-top: var(--space-3);
    color: var(--text-faint);
    font-size: var(--text-xs);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-4);
  }
  .actions button {
    height: 36px;
    padding: 0 var(--space-4);
    border-radius: var(--radius-sm);
    font-weight: 550;
    cursor: pointer;
  }
  .secondary {
    border: 1px solid var(--line-strong);
    background: transparent;
  }
  .secondary:hover {
    background: var(--bg-hover);
  }
  .confirm {
    border: 0;
    background: var(--accent);
    color: var(--on-accent);
  }
  .confirm:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
