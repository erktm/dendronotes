<script lang="ts" module>
  import type { IconName } from './Icon.svelte'

  export interface MenuItem {
    label: string
    icon: IconName
    action: () => void
    danger?: boolean
    /** One of several choices: shows a check when chosen (the icon is not shown). */
    checked?: boolean
    /** Starts a new group (a line above). */
    separator?: boolean
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
      role={item.checked === undefined ? 'menuitem' : 'menuitemradio'}
      aria-checked={item.checked}
      class:danger={item.danger}
      class:sep={item.separator}
      onclick={() => {
        onclose()
        item.action()
      }}
    >
      {#if item.checked === undefined}
        <Icon name={item.icon} size={16} />
      {:else if item.checked}
        <Icon name="check" size={16} />
      {:else}
        <span class="blank"></span>
      {/if}
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
  .sep {
    position: relative;
    margin-top: 9px;
  }
  .sep::before {
    position: absolute;
    top: -5px;
    right: 4px;
    left: 4px;
    border-top: 1px solid var(--line);
    content: '';
  }
  .blank {
    flex: none;
    width: 16px;
  }
  .header {
    margin: 0 0 5px;
    padding: 6px var(--space-3) 10px;
    border-bottom: 1px solid var(--line);
  }
</style>
