<script lang="ts">
  import type { Peer } from '../lib/api/collab'
  import { strings } from '../lib/strings'

  let { peers }: { peers: readonly Peer[] } = $props()

  /** One letter or two, from the name. */
  function initials(name: string): string {
    const parts = name.trim().split(/[\s._@-]+/).filter(Boolean)
    const letters = parts.length > 1 ? [parts[0]![0], parts[1]![0]] : [...(parts[0] ?? '?')].slice(0, 2)
    return letters.join('').toUpperCase()
  }

  const shown = $derived(peers.slice(0, 4))
</script>

{#if peers.length}
  <div class="peers" role="group" aria-label={strings.collab.peers(peers.length)}>
    {#each shown as p (p.id)}
      <span class="avatar peer-c{((p.color % 8) + 8) % 8}" title={strings.collab.peer(p.name)} aria-label={p.name}>{initials(p.name)}</span>
    {/each}
    {#if peers.length > shown.length}
      <span class="avatar more" title={strings.collab.peers(peers.length)}>+{peers.length - shown.length}</span>
    {/if}
  </div>
{/if}

<style>
  .peers {
    display: flex;
    align-items: center;
    padding-left: 6px;
  }
  .avatar {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    margin-left: -6px;
    border: 2px solid var(--bg);
    border-radius: 50%;
    background: var(--peer);
    color: var(--on-peer);
    font-size: 10.5px;
    font-weight: 700;
    letter-spacing: 0.02em;
    cursor: default;
    user-select: none;
  }
  .more {
    background: var(--bg-sunken);
    color: var(--text-muted);
  }
</style>
