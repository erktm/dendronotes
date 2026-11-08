<script lang="ts">
  import { closeOnBackdrop } from '../lib/actions'
  import { app } from '../lib/state/app.svelte'
  import { strings } from '../lib/strings'

  let dialog: HTMLDialogElement | undefined = $state()
  let input: HTMLInputElement | undefined = $state()
  let value = $state('')

  $effect(() => {
    if (!dialog) return
    const req = app.promptRequest
    if (req && !dialog.open) {
      value = req.value
      dialog.showModal()
      queueMicrotask(() => {
        input?.focus()
        input?.select()
      })
    } else if (!req && dialog.open) dialog.close()
  })

  function submit(e: SubmitEvent) {
    e.preventDefault()
    if (value.trim()) app.answerPrompt(value.trim())
  }
</script>

<dialog
  bind:this={dialog}
  use:closeOnBackdrop
  aria-labelledby="prompt-title"
  onclose={() => {
    if (app.promptRequest) app.answerPrompt(null)
  }}
>
  {#if app.promptRequest}
    {@const req = app.promptRequest}
    <form onsubmit={submit}>
      <h2 id="prompt-title">{req.title}</h2>
      {#if req.text}<p>{req.text}</p>{/if}
      <label for="prompt-input">{req.label}</label>
      <input id="prompt-input" bind:this={input} bind:value placeholder={req.placeholder} maxlength="100" autocomplete="off" />
      <div class="actions">
        <button type="button" class="secondary" onclick={() => app.answerPrompt(null)}>{strings.dialogs.cancel}</button>
        <button type="submit" class="confirm" disabled={!value.trim()}>{req.confirmLabel}</button>
      </div>
    </form>
  {/if}
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
  form {
    display: flex;
    flex-direction: column;
  }
  h2 {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  p {
    margin: var(--space-2) 0 0;
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  label {
    margin: var(--space-4) 0 var(--space-2);
    font-size: var(--text-sm);
    font-weight: 550;
  }
  input {
    height: 40px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
  }
  input:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
    border-color: transparent;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-6);
  }
  button {
    height: 36px;
    padding: 0 var(--space-4);
    border-radius: var(--radius-sm);
    font: inherit;
    font-weight: 550;
    cursor: pointer;
  }
  .secondary {
    border: 1px solid var(--line);
    background: transparent;
    color: var(--text);
  }
  .confirm {
    border: 0;
    background: var(--accent);
    color: var(--on-accent);
  }
  .confirm:disabled {
    cursor: not-allowed;
    opacity: 0.5;
  }
</style>
