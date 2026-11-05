<script lang="ts">
  import { app } from '../lib/state/app.svelte'

  let dialog: HTMLDialogElement | undefined = $state()

  $effect(() => {
    if (!dialog) return
    if (app.confirmRequest && !dialog.open) dialog.showModal()
    else if (!app.confirmRequest && dialog.open) dialog.close()
  })
</script>

<dialog
  bind:this={dialog}
  aria-labelledby="confirm-title"
  aria-describedby="confirm-text"
  onclose={() => {
    if (app.confirmRequest) app.answer(false)
  }}
>
  {#if app.confirmRequest}
    {@const req = app.confirmRequest}
    <h2 id="confirm-title">{req.title}</h2>
    <p id="confirm-text">{req.message}</p>
    <div class="actions">
      <button type="button" class="secondary" onclick={() => app.answer(false)}>Cancel</button>
      <button type="button" class="confirm" class:danger={req.danger} onclick={() => app.answer(true)}>
        {req.confirmLabel}
      </button>
    </div>
  {/if}
</dialog>

<style>
  dialog {
    width: min(92vw, 420px);
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
    font-size: var(--text-lg);
    font-weight: 600;
  }
  p {
    margin: var(--space-2) 0 var(--space-6);
    color: var(--text-muted);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  button {
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
  .confirm.danger {
    background: var(--err);
    color: #fff;
  }
</style>
