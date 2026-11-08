<script lang="ts" module>
  import type { IconName } from './Icon.svelte'

  export interface MenuItem {
    label: string
    icon: IconName
    action: () => void
    danger?: boolean
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import Icon from './Icon.svelte'
  import { cssVars } from '../lib/actions'

  let {
    x,
    y,
    items,
    onclose,
    header,
  }: { x: number; y: number; items: MenuItem[]; onclose: () => void; header?: Snippet } = $props()

  let menu: HTMLDivElement | undefined = $state()
  let pos = $state({ left: -9999, top: -9999 })

  // Keep the menu inside the viewport and focus the first entry.
  $effect(() => {
    if (!menu) return
    const r = menu.getBoundingClientRect()
    pos = {
      left: Math.max(8, Math.min(x, window.innerWidth - r.width - 8)),
      top: Math.max(8, Math.min(y, window.innerHeight - r.height - 8)),
    }
    menu.querySelector<HTMLButtonElement>('button')?.focus()
  })

  function onkeydown(e: KeyboardEvent) {
    if (!menu) return
    const buttons = [...menu.querySelectorAll<HTMLButtonElement>('button')]
    const i = buttons.indexOf(document.activeElement as HTMLButtonElement)
    if (e.key === 'Escape' || e.key === 'Tab') {
      e.preventDefault()
      onclose()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      buttons[(i + 1) % buttons.length]?.focus()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      buttons[(i - 1 + buttons.length) % buttons.length]?.focus()
    }
  }

  function onpointerdown(e: PointerEvent) {
    if (menu && !menu.contains(e.target as Node)) onclose()
  }
</script>

<svelte:window {onpointerdown} onblur={onclose} onresize={onclose} />

<div
  class="menu"
  role="menu"
  tabindex="-1"
  bind:this={menu}
  use:cssVars={{ left: `${pos.left}px`, top: `${pos.top}px` }}
  {onkeydown}
>
  {#if header}
    <div class="header">{@render header()}</div>
  {/if}
  {#each items as item (item.label)}
    <button
      type="button"
      role="menuitem"
      class:danger={item.danger}
      onclick={() => {
        onclose()
        item.action()
      }}
    >
      <Icon name={item.icon} size={16} />
      {item.label}
    </button>
  {/each}
</div>

<style>
  .menu {
    position: fixed;
    z-index: 50;
    min-width: 190px;
    padding: 5px;
    border: 1px solid var(--line);
    border-radius: var(--radius);
    background: var(--bg-raised);
    box-shadow: var(--shadow), 0 12px 32px rgb(0 0 0 / 0.12);
  }
  button {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    height: 32px;
    padding: 0 var(--space-3);
    border: 0;
    border-radius: 6px;
    background: transparent;
    font-size: var(--text-sm);
    text-align: left;
    cursor: pointer;
  }
  button:hover,
  button:focus-visible {
    background: var(--bg-hover);
    outline: none;
  }
  .danger {
    color: var(--err);
  }
  .header {
    margin: 0 0 5px;
    padding: 6px var(--space-3) 10px;
    border-bottom: 1px solid var(--line);
  }
</style>
