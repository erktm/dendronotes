<script lang="ts">
  import type { ServerState } from '../lib/api/health'

  let { state }: { state: ServerState } = $props()

  const labels: Record<ServerState, string> = {
    checking: 'Connecting …',
    ready: 'Connected',
    unavailable: 'Server not ready',
    offline: 'No connection',
  }
</script>

<span class="chip {state}" role="status">
  <span class="dot" aria-hidden="true"></span>
  {labels[state]}
</span>

<style>
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: 3px var(--space-3) 3px var(--space-2);
    border: 1px solid var(--line);
    border-radius: 999px;
    font-size: var(--text-xs);
    color: var(--text-muted);
    background: var(--bg-raised);
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--text-faint);
  }
  .ready .dot {
    background: var(--ok);
  }
  .unavailable .dot {
    background: var(--warn);
  }
  .offline .dot {
    background: var(--err);
  }
</style>
