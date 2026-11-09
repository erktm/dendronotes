<script lang="ts">
  import Avatar from './Avatar.svelte'
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { live } from '../lib/state/live.svelte'
  import { notifications } from '../lib/state/notifications.svelte'
  import { comments } from '../lib/state/comments.svelte'
  import { router } from '../lib/router.svelte'
  import type { Notification } from '../lib/api/comments'
  import { displayName } from '../lib/paths'
  import { ago } from '../lib/time'
  import { strings } from '../lib/strings'

  const t = strings.notifications

  let open = $state(false)
  let root: HTMLDivElement | undefined = $state()

  // On start, when told so, and after reconnecting.
  $effect(() => {
    void live.notificationVersion
    if (live.state === 'live' || !notifications.loaded) void notifications.load()
  })

  $effect(() => {
    if (!open) return
    const outside = (e: PointerEvent) => {
      if (root && !root.contains(e.target as Node)) open = false
    }
    const escape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        open = false
        root?.querySelector<HTMLButtonElement>('.bell')?.focus()
      }
    }
    document.addEventListener('pointerdown', outside)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('pointerdown', outside)
      document.removeEventListener('keydown', escape)
    }
  })

  function toggle() {
    open = !open
    if (open) void notifications.load()
  }

  const who = (n: Notification) => (n.actor ? n.actor.displayName || n.actor.username : t.someone)
  const what = (n: Notification) => (n.kind === 'mention' ? t.mention : t.reply)(who(n), displayName(n.path))

  function go(n: Notification) {
    open = false
    if (!n.read) void notifications.read([n.id])
    if (n.somaId !== app.somaId || router.notePath !== n.path || router.view !== 'soma') router.goNote(n.somaId, n.path)
    app.commentsOpen = true
    comments.show(n.threadId)
  }
</script>

<div class="wrap" bind:this={root}>
  <button
    type="button"
    class="bell"
    aria-haspopup="dialog"
    aria-expanded={open}
    aria-label={notifications.unread ? t.openUnread(notifications.unread) : t.open}
    title={notifications.unread ? t.openUnread(notifications.unread) : t.open}
    onclick={toggle}
  >
    <Icon name="bell" size={18} />
    {#if notifications.unread}<span class="badge" aria-hidden="true">{notifications.unread > 9 ? '9+' : notifications.unread}</span>{/if}
  </button>
  {#if open}
    <div class="popover" role="dialog" aria-label={t.title}>
      <header>
        <h2>{t.title}</h2>
        {#if notifications.unread}
          <button type="button" class="link" onclick={() => void notifications.read()}>{t.markAll}</button>
        {/if}
      </header>
      {#if notifications.list.length === 0}
        <p class="none">{t.none}</p>
      {:else}
        <ul>
          {#each notifications.list as n (n.id)}
            <li>
              <button type="button" class="item" class:unread={!n.read} onclick={() => go(n)}>
                {#if n.actor}
                  <Avatar id={n.actor.id} name={who(n)} avatar={n.actor.avatar ?? undefined} size={28} />
                {:else}
                  <span class="ghost" aria-hidden="true"><Icon name="user" size={14} /></span>
                {/if}
                <span class="body">
                  <span class="what">{what(n)}</span>
                  <span class="excerpt">{n.excerpt}</span>
                  <span class="where">{t.inSoma(n.somaName, ago(n.createdAt))}</span>
                </span>
                {#if !n.read}<span class="dot" aria-hidden="true"></span>{/if}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>

<style>
  .wrap {
    position: relative;
  }
  .bell {
    position: relative;
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .bell:hover,
  .bell[aria-expanded='true'] {
    background: var(--bg-hover);
    color: var(--text);
  }
  .badge {
    position: absolute;
    top: 2px;
    right: 1px;
    min-width: 16px;
    height: 16px;
    padding: 0 4px;
    border-radius: 999px;
    background: var(--accent);
    color: var(--on-accent);
    font-size: 10px;
    font-weight: 700;
    line-height: 16px;
  }
  .popover {
    position: absolute;
    z-index: 30;
    top: calc(100% + 6px);
    right: -40px;
    width: min(92vw, 360px);
    max-height: 70vh;
    overflow: auto;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--bg-raised);
    box-shadow: var(--shadow), 0 16px 40px rgb(0 0 0 / 0.18);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 14px 8px;
    border-bottom: 1px solid var(--line);
  }
  h2 {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .link {
    border: 0;
    background: transparent;
    color: var(--accent-strong);
    font-size: var(--text-xs);
    font-weight: 600;
    cursor: pointer;
  }
  .none {
    padding: 18px 16px 22px;
    color: var(--text-faint);
    font-size: var(--text-sm);
  }
  ul {
    margin: 0;
    padding: 6px;
    list-style: none;
  }
  .item {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    width: 100%;
    padding: 8px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    text-align: left;
    cursor: pointer;
  }
  .item:hover {
    background: var(--bg-hover);
  }
  .ghost {
    display: grid;
    flex: none;
    place-items: center;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--bg-sunken);
    color: var(--text-faint);
  }
  .body {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .what {
    font-size: var(--text-sm);
    font-weight: 550;
  }
  .unread .what {
    font-weight: 700;
  }
  .excerpt {
    display: -webkit-box;
    overflow: hidden;
    color: var(--text-muted);
    font-size: var(--text-xs);
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow-wrap: anywhere;
  }
  .where {
    color: var(--text-faint);
    font-size: var(--text-xs);
  }
  .dot {
    flex: none;
    width: 8px;
    height: 8px;
    margin-top: 6px;
    border-radius: 50%;
    background: var(--accent);
  }
</style>
