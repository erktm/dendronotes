<script lang="ts">
  import Avatar from './Avatar.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import { app } from '../lib/state/app.svelte'
  import { strings } from '../lib/strings'

  let button: HTMLButtonElement | undefined = $state()
  let at = $state<{ x: number; y: number } | null>(null)

  const user = $derived(app.user)
  const name = $derived(user ? user.displayName || user.username : '')

  const items: MenuItem[] = [
    { label: strings.auth.profileItem, icon: 'user', action: () => (app.profileOpen = true) },
    { label: strings.auth.signOut, icon: 'logout', action: () => void app.logout() },
  ]

  function toggle() {
    if (at || !button) {
      at = null
      return
    }
    // Below the button, right-aligned (the menu keeps itself in the window).
    const r = button.getBoundingClientRect()
    at = { x: r.right, y: r.bottom + 6 }
  }

  function close() {
    at = null
    button?.focus()
  }
</script>

{#if user}
  <button
    bind:this={button}
    type="button"
    class="me"
    aria-haspopup="menu"
    aria-expanded={at !== null}
    aria-label={strings.auth.account(name)}
    title={strings.auth.signedInAs(user.username)}
    onpointerdown={(e) => e.stopPropagation()}
    onclick={toggle}
  >
    <Avatar id={user.id} {name} avatar={user.avatar} size={28} />
  </button>
  {#if at}
    <ContextMenu x={at.x} y={at.y} {items} onclose={close}>
      {#snippet header()}
        <div class="who">
          <Avatar id={user.id} {name} avatar={user.avatar} size={38} />
          <span class="names">
            <span class="name">{name}</span>
            <span class="username">{user.username}</span>
          </span>
        </div>
      {/snippet}
    </ContextMenu>
  {/if}
{/if}

<style>
  .me {
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    margin-left: 4px;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: transparent;
    cursor: pointer;
  }
  .me:hover :global(.avatar),
  .me[aria-expanded='true'] :global(.avatar) {
    box-shadow: 0 0 0 2px var(--bg), 0 0 0 4px var(--line-strong);
  }
  .me:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
  }
  .who {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .names {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .name {
    overflow: hidden;
    color: var(--text);
    font-size: var(--text-sm);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .username {
    overflow: hidden;
    color: var(--text-muted);
    font-size: var(--text-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
