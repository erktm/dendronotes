<script lang="ts">
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { router } from '../lib/router.svelte'
  import { strings } from '../lib/strings'
  import type { Membership } from '../lib/api/soma'

  let open = $state(false)
  let root: HTMLDivElement | undefined = $state()
  let menu: HTMLDivElement | undefined = $state()

  const mine = $derived(app.somas.filter((s) => s.role === 'owner'))
  const shared = $derived(app.somas.filter((s) => s.role !== 'owner'))
  const roleLabel = (m: Membership) => (m.role === 'editor' ? strings.somas.editor : strings.somas.viewer)

  function toggle() {
    open = !open
    if (open) queueMicrotask(() => menu?.querySelector<HTMLButtonElement>('[aria-checked="true"], button')?.focus())
  }

  function close(refocus = true) {
    open = false
    if (refocus) root?.querySelector<HTMLButtonElement>('.current')?.focus()
  }

  function choose(id: string) {
    close()
    if (id !== app.somaId) router.goSoma(id)
  }

  async function create() {
    close(false)
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

  async function rename() {
    close(false)
    const soma = app.soma
    if (!soma) return
    const name = await app.prompt({
      title: strings.somas.renameTitle,
      label: strings.somas.nameLabel,
      value: soma.name,
      confirmLabel: strings.somas.renameButton,
    })
    if (name && name !== soma.name) await app.renameSoma(soma.id, name)
  }

  function onkeydown(e: KeyboardEvent) {
    if (!open || !menu) return
    const items = [...menu.querySelectorAll<HTMLButtonElement>('button')]
    const i = items.indexOf(document.activeElement as HTMLButtonElement)
    if (e.key === 'Escape') {
      e.preventDefault()
      close()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      items[(i + 1) % items.length]?.focus()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      items[(i - 1 + items.length) % items.length]?.focus()
    }
  }

  function onpointerdown(e: PointerEvent) {
    if (open && root && !root.contains(e.target as Node)) close(false)
  }
</script>

<svelte:window {onpointerdown} />

<div class="switcher" bind:this={root}>
  <button
    type="button"
    class="current"
    aria-haspopup="menu"
    aria-expanded={open}
    title={strings.somas.switch}
    onclick={toggle}
  >
    <span class="dot" aria-hidden="true"></span>
    <span class="name">{app.soma?.name ?? strings.somas.switch}</span>
    {#if app.soma && app.soma.role !== 'owner'}<span class="badge">{roleLabel(app.soma)}</span>{/if}
    <span class="chev" aria-hidden="true"><Icon name="chevron" size={14} /></span>
  </button>

  {#if open}
    <!-- svelte-ignore a11y_interactive_supports_focus -->
    <div class="menu" role="menu" tabindex="-1" aria-label={strings.somas.switch} bind:this={menu} {onkeydown}>
      {#if mine.length}
        <p class="group">{strings.somas.mine}</p>
        {#each mine as s (s.id)}
          <button type="button" role="menuitemradio" aria-checked={s.id === app.somaId} onclick={() => choose(s.id)}>
            <span class="check" aria-hidden="true">{s.id === app.somaId ? '✓' : ''}</span>
            <span class="label">{s.name}</span>
          </button>
        {/each}
      {/if}
      {#if shared.length}
        <p class="group">{strings.somas.shared}</p>
        {#each shared as s (s.id)}
          <button type="button" role="menuitemradio" aria-checked={s.id === app.somaId} onclick={() => choose(s.id)}>
            <span class="check" aria-hidden="true">{s.id === app.somaId ? '✓' : ''}</span>
            <span class="label">{s.name}</span>
            <span class="meta">{strings.somas.ownedBy(s.owner.displayName || s.owner.username)}</span>
          </button>
        {/each}
      {/if}
      <hr />
      <button type="button" role="menuitem" onclick={create}>
        <span class="check" aria-hidden="true"><Icon name="plus" size={14} /></span>
        <span class="label">{strings.somas.create}</span>
      </button>
      {#if app.soma?.role === 'owner'}
        <button type="button" role="menuitem" onclick={rename}>
          <span class="check" aria-hidden="true"><Icon name="pencil" size={14} /></span>
          <span class="label">{strings.somas.rename}</span>
        </button>
        <button type="button" role="menuitem" class="danger" onclick={() => (close(false), void app.deleteSoma(app.somaId))}>
          <span class="check" aria-hidden="true"><Icon name="trash" size={14} /></span>
          <span class="label">{strings.somas.delete}</span>
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .switcher {
    position: relative;
    padding: var(--space-2) var(--space-2) 0;
  }
  .current {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    height: 34px;
    padding: 0 8px 0 10px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
    font-weight: 600;
    text-align: left;
    cursor: pointer;
  }
  .current:hover {
    border-color: var(--line-strong);
  }
  .dot {
    width: 8px;
    height: 8px;
    flex: none;
    border-radius: 50%;
    background: var(--accent);
  }
  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .badge {
    flex: none;
    padding: 1px 6px;
    border-radius: 999px;
    background: var(--bg-sunken);
    color: var(--text-muted);
    font-size: 11px;
    font-weight: 550;
  }
  .chev {
    display: grid;
    flex: none;
    color: var(--text-muted);
    transform: rotate(90deg);
  }
  .menu {
    position: absolute;
    z-index: 20;
    top: calc(100% + 4px);
    right: var(--space-2);
    left: var(--space-2);
    max-height: 60vh;
    padding: 4px;
    overflow: auto;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--bg-raised);
    box-shadow: var(--shadow), 0 12px 32px rgb(0 0 0 / 0.16);
  }
  .group {
    margin: 6px 8px 2px;
    color: var(--text-faint);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .menu button {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    min-height: 32px;
    padding: 4px 8px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
    text-align: left;
    cursor: pointer;
  }
  .menu button:hover,
  .menu button:focus-visible {
    background: var(--bg-hover);
    outline: none;
  }
  .check {
    display: grid;
    width: 14px;
    flex: none;
    place-items: center;
    color: var(--accent);
  }
  .label {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .meta {
    flex: none;
    color: var(--text-faint);
    font-size: var(--text-xs);
  }
  .danger {
    color: var(--err) !important;
  }
  hr {
    margin: 4px 2px;
    border: 0;
    border-top: 1px solid var(--line);
  }
</style>
