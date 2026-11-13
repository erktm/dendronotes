<script lang="ts">
  // What dendronotes is, its version and where its source code lives.
  import Icon from './Icon.svelte'
  import Logo from './Logo.svelte'
  import Wordmark from './Wordmark.svelte'
  import { closeOnBackdrop } from '../lib/actions'
  import { app } from '../lib/state/app.svelte'
  import { brand, strings } from '../lib/strings'

  const t = strings.about
  let dialog: HTMLDialogElement | undefined = $state()

  $effect(() => {
    if (dialog && !dialog.open) dialog.showModal()
  })
</script>

<dialog bind:this={dialog} use:closeOnBackdrop aria-labelledby="about-title" onclose={() => (app.aboutOpen = false)}>
  <button type="button" class="close" aria-label={t.close} onclick={() => dialog?.close()}><Icon name="close" size={18} /></button>
  <div class="brand">
    <Logo size={44} />
    <h2 id="about-title"><Wordmark size="lg" /></h2>
  </div>
  {#if app.version}<p class="version">{t.version(app.version)}</p>{/if}
  <p>{t.text}</p>
  <p class="faint">{t.license}</p>
  <a class="source" href={brand.repository} target="_blank" rel="noopener noreferrer">
    <Icon name="external" size={16} />
    {t.source}
  </a>
</dialog>

<style>
  dialog {
    position: relative;
    width: min(92vw, 420px);
    padding: var(--space-6);
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow), 0 24px 64px rgb(0 0 0 / 0.2);
    text-align: center;
  }
  dialog::backdrop {
    background: var(--scrim);
  }
  .close {
    position: absolute;
    top: 10px;
    right: 10px;
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .close:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .brand {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }
  h2 {
    margin: 0;
  }
  .version {
    margin-bottom: var(--space-3);
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-sm);
  }
  p {
    margin: 0 0 var(--space-3);
    line-height: 1.55;
  }
  .faint {
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .source {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-top: var(--space-2);
    padding: 8px 14px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    color: var(--text);
    font-weight: 550;
    text-decoration: none;
  }
  .source:hover {
    background: var(--bg-hover);
  }
</style>
