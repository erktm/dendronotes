<script lang="ts">
  // Links to one note or drawing: the internal link everyone with access can
  // copy, and public read-only links, which editors create and revoke.
  import Icon from './Icon.svelte'
  import { closeOnBackdrop } from '../lib/actions'
  import { app } from '../lib/state/app.svelte'
  import * as links from '../lib/api/links'
  import type { ShareLink } from '../lib/api/links'
  import { formatDate } from '../lib/api/admin'
  import { isDrawingPath } from '../lib/drawing/paths'
  import { errorMessage } from '../lib/errors'
  import { displayName, isNote } from '../lib/paths'
  import { strings } from '../lib/strings'

  let { path }: { path: string } = $props()

  const t = strings.links
  const DAYS = [0, 1, 7, 30, 90, 365]

  let dialog: HTMLDialogElement | undefined = $state()
  let all = $state.raw<ShareLink[]>([])
  let days = $state(30)
  let fresh = $state<string | null>(null)
  let busy = $state(false)

  const shareable = $derived(isNote(path) || isDrawingPath(path))
  const editor = $derived(app.can.organize)
  const mine = $derived(all.filter((l) => l.path === path))
  const others = $derived(all.filter((l) => l.path !== path))
  const now = Date.now()
  const expired = (l: ShareLink) => !!l.expiresAt && new Date(l.expiresAt).getTime() <= now
  const by = (l: ShareLink) => (l.createdBy ? l.createdBy.displayName || l.createdBy.username : null)

  $effect(() => {
    if (dialog && !dialog.open) dialog.showModal()
  })

  $effect(() => {
    if (editor && shareable) void load()
  })

  async function load() {
    try {
      all = await links.publicLinks()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      app.toast(t.copied, 'info')
    } catch {
      app.toast(t.copyFailed)
    }
  }

  async function copyInternal() {
    try {
      const l = await links.internalLink(path)
      await copy(links.internalURL(app.somaId, l.id))
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function create() {
    if (busy) return
    busy = true
    try {
      const r = await links.createPublicLink(path, days)
      fresh = links.publicURL(r.token)
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  async function revoke(l: ShareLink) {
    try {
      await links.revokePublicLink(l.id)
      app.toast(t.revoked, 'info')
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }
</script>

{#snippet row(l: ShareLink, showPath: boolean)}
  <li class:expired={expired(l)}>
    <span class="what">
      {#if showPath}<strong>{l.path}</strong>{/if}
      <span class="faint">
        {t.meta(formatDate(l.createdAt), by(l))}{#if l.expiresAt}, {expired(l) ? t.expired : t.expires(formatDate(l.expiresAt))}{/if}
      </span>
    </span>
    <button type="button" class="button small" aria-label={t.revokeLabel(l.path)} onclick={() => void revoke(l)}>{t.revoke}</button>
  </li>
{/snippet}

<dialog bind:this={dialog} use:closeOnBackdrop aria-labelledby="link-title" onclose={() => (app.linkDialog = null)}>
  <header>
    <h2 id="link-title">{t.title(displayName(path))}</h2>
    <button type="button" class="icon-btn" aria-label={t.close} onclick={() => dialog?.close()}>
      <Icon name="close" size={18} />
    </button>
  </header>

  <section class="section">
    <h3>{t.internal}</h3>
    <p class="hint">{t.internalHint}</p>
    <div class="row">
      <button type="button" class="button" onclick={() => void copyInternal()}>
        <Icon name="copy" size={15} />
        {t.copy}
      </button>
    </div>
  </section>

  {#if shareable}
    <section class="section">
      <h3>{t.public}</h3>
      <p class="hint">{t.publicHint}</p>
      {#if editor}
        <div class="row">
          <label>
            <span>{t.expiry}</span>
            <select bind:value={days}>
              {#each DAYS as d (d)}<option value={d}>{t.expiries[d]}</option>{/each}
            </select>
          </label>
          <button type="button" class="button primary" disabled={busy} onclick={() => void create()}>
            <Icon name="external" size={15} />
            {t.create}
          </button>
        </div>
        {#if fresh}
          <div class="fresh" role="status">
            <p>{t.newLink}</p>
            <div class="code">
              <input type="text" readonly value={fresh} aria-label={t.public} onfocus={(e) => e.currentTarget.select()} />
              <button type="button" class="button" onclick={() => void copy(fresh!)}>
                <Icon name="copy" size={15} />
                {t.copy}
              </button>
            </div>
          </div>
        {/if}
        {#if mine.length}
          <ul class="links">
            {#each mine as l (l.id)}{@render row(l, false)}{/each}
          </ul>
        {:else}
          <p class="hint">{t.none}</p>
        {/if}
        {#if others.length}
          <details>
            <summary>{t.others(others.length)}</summary>
            <ul class="links">
              {#each others as l (l.id)}{@render row(l, true)}{/each}
            </ul>
          </details>
        {/if}
      {:else}
        <p class="hint">{t.onlyEditors}</p>
      {/if}
    </section>
  {/if}
</dialog>

<style>
  dialog {
    width: min(92vw, 540px);
    max-height: 90vh;
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
    justify-content: space-between;
    gap: 12px;
    padding: 16px 16px 16px 20px;
    border-bottom: 1px solid var(--line);
  }
  h2 {
    min-width: 0;
    overflow: hidden;
    font-size: var(--text-lg);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  h3 {
    margin: 0 0 4px;
    font-size: 11.5px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .section {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 16px 20px 18px;
  }
  .section + .section {
    border-top: 1px solid var(--line);
  }
  .hint,
  .faint {
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: 10px;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: var(--text-sm);
    font-weight: 550;
  }
  select,
  input {
    height: 34px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
  }
  .button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 34px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    font-size: var(--text-sm);
    font-weight: 550;
    cursor: pointer;
  }
  .button:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .button.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--on-accent);
  }
  .button.small {
    height: 28px;
  }
  .button:disabled {
    opacity: 0.5;
  }
  .icon-btn {
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
  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .fresh {
    padding: 10px 12px;
    border: 1px solid var(--accent);
    border-radius: var(--radius-sm);
    background: var(--accent-soft);
    font-size: var(--text-sm);
  }
  .fresh .code {
    display: flex;
    gap: 8px;
    margin-top: 8px;
  }
  .fresh input {
    flex: 1;
    min-width: 0;
    font-family: var(--font-mono);
  }
  .links {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .links li {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
  }
  .links li.expired {
    opacity: 0.6;
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
    font-weight: 550;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  details summary {
    color: var(--text-muted);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  details .links {
    margin-top: 8px;
  }
</style>
