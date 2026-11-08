<script lang="ts">
  import Avatar from './Avatar.svelte'
  import type { Peer } from '../lib/api/collab'
  import { strings } from '../lib/strings'

  let { peers }: { peers: readonly Peer[] } = $props()

  const shown = $derived(peers.slice(0, 4))
</script>

{#if peers.length}
  <div class="peers" role="group" aria-label={strings.collab.peers(peers.length)}>
    {#each shown as p (p.id)}
      <!-- The ring has the color of their cursor in the note. -->
      <span class="ring peer-c{((p.color % 8) + 8) % 8}" title={strings.collab.peer(p.name)}>
        <Avatar id={p.user} name={p.name} avatar={p.avatar} size={22} label={p.name} />
      </span>
    {/each}
    {#if peers.length > shown.length}
      <span class="more" title={strings.collab.peers(peers.length)}>+{peers.length - shown.length}</span>
    {/if}
  </div>
{/if}

<style>
  .peers {
    display: flex;
    align-items: center;
    padding-left: 6px;
  }
  .ring {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    margin-left: -6px;
    border: 2px solid var(--peer);
    border-radius: 50%;
    background: var(--bg);
    cursor: default;
  }
  .more {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    margin-left: -6px;
    border: 2px solid var(--bg);
    border-radius: 50%;
    background: var(--bg-sunken);
    color: var(--text-muted);
    font-size: 10.5px;
    font-weight: 700;
    cursor: default;
    user-select: none;
  }
</style>
