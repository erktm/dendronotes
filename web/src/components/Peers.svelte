<script lang="ts">
  // The others in the open note, for the note's status bar: their pictures,
  // each in a ring of their cursor color, and who they are.
  import Avatar from './Avatar.svelte'
  import type { Peer } from '../lib/api/collab'
  import { strings } from '../lib/strings'

  let { peers }: { peers: readonly Peer[] } = $props()

  const shown = $derived(peers.slice(0, 5))
  // One name per account, also when someone has the note open twice.
  const names = $derived([...new Map(peers.map((p) => [p.user, p.name])).values()])
</script>

{#if peers.length}
  <div class="peers" role="group" aria-label={strings.collab.peers(peers.length)}>
    <span class="faces">
      {#each shown as p (p.id)}
        <!-- The ring has the color of their cursor in the note. -->
        <span class="ring peer-c{((p.color % 8) + 8) % 8}" title={strings.collab.peer(p.name)}>
          <Avatar id={p.user} name={p.name} avatar={p.avatar} size={18} label={p.name} />
        </span>
      {/each}
      {#if peers.length > shown.length}
        <span class="more" title={strings.collab.peers(peers.length)}>+{peers.length - shown.length}</span>
      {/if}
    </span>
    <span class="names">{strings.collab.here(names)}</span>
  </div>
{/if}

<style>
  .peers {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .faces {
    display: flex;
    flex: none;
    align-items: center;
    padding-left: 5px;
  }
  .ring,
  .more {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    margin-left: -5px;
    border-radius: 50%;
    cursor: default;
  }
  .ring {
    border: 2px solid var(--peer);
    background: var(--bg);
  }
  .more {
    border: 2px solid var(--bg);
    background: var(--bg-sunken);
    color: var(--text-muted);
    font-size: 9.5px;
    font-weight: 700;
    user-select: none;
  }
  .names {
    overflow: hidden;
    color: var(--text-muted);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
