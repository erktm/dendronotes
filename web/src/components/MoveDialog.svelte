<script lang="ts">
  // Picks the folder some entries move to: every folder of the soma except
  // the moving ones (and what lies inside them).
  import Icon from './Icon.svelte'
  import { closeOnBackdrop, cssVars } from '../lib/actions'
  import { app } from '../lib/state/app.svelte'
  import { basename, isInside } from '../lib/paths'
  import { strings } from '../lib/strings'
  import { outermost } from '../lib/tree'

  let { paths, onclose }: { paths: readonly string[]; onclose: () => void } = $props()

  let dialog: HTMLDialogElement | undefined = $state()
  let target = $state<string | null>(null)
  let busy = $state(false)

  const moving = $derived(outermost(paths))
  const folders = $derived(
    app.entries
      .filter((e) => e.type === 'dir' && !moving.some((p) => isInside(e.path, p)))
      .map((e) => e.path)
      .sort((a, b) => a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' })),
  )

  $effect(() => {
    if (dialog && !dialog.open) dialog.showModal()
  })

  async function move() {
    if (target === null || busy) return
    busy = true
    await app.moveMany(moving, target)
    busy = false
    dialog?.close()
  }
</script>

<dialog bind:this={dialog} use:closeOnBackdrop aria-labelledby="move-title" {onclose}>
  <h2 id="move-title">{strings.tree.moveToTitle(moving.length)}</h2>
  <div class="list" role="listbox" aria-labelledby="move-title">
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
          void move()
        }}
      >
        <Icon name={f ? 'folder' : 'folder-open'} size={16} />
        <span>{f ? basename(f) : strings.tree.topLevel}</span>
      </button>
    {/each}
  </div>
  <div class="actions">
    <button type="button" class="secondary" onclick={() => dialog?.close()}>{strings.dialogs.cancel}</button>
    <button type="button" class="confirm" disabled={target === null || busy} onclick={move}>{strings.tree.move}</button>
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
    background: rgb(10 20 14 / 0.35);
  }
  h2 {
    margin-bottom: var(--space-3);
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .list {
    max-height: min(50vh, 420px);
    overflow: auto;
    padding: 4px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
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
