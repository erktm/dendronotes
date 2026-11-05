<script lang="ts">
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
</script>

<div class="toasts" role="status" aria-live="polite">
  {#each app.toasts as t (t.id)}
    <div class="toast {t.kind}">
      {#if t.kind === 'error'}<Icon name="alert" size={17} />{/if}
      <span>{t.text}</span>
      <button type="button" aria-label="Dismiss" onclick={() => app.dismiss(t.id)}>
        <Icon name="close" size={15} />
      </button>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    right: var(--space-4);
    bottom: var(--space-4);
    z-index: 60;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    max-width: min(420px, calc(100vw - 32px));
  }
  .toast {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-3) var(--space-3) var(--space-4);
    border: 1px solid var(--line);
    border-radius: var(--radius);
    background: var(--bg-raised);
    box-shadow: var(--shadow);
    font-size: var(--text-sm);
  }
  .toast.error {
    border-color: color-mix(in srgb, var(--err) 45%, var(--line));
  }
  .toast.error :global(.icon) {
    margin-top: 1px;
    color: var(--err);
  }
  span {
    flex: 1;
  }
  button {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  button:hover {
    background: var(--bg-hover);
  }
</style>
