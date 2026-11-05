<script lang="ts">
  import Logo from './Logo.svelte'

  let { filesOpen, ontogglefiles }: { filesOpen: boolean; ontogglefiles: () => void } = $props()
</script>

<nav class="rail" aria-label="Main navigation">
  <div class="brand" title="leafnotes"><Logo size={26} /></div>

  <button
    class="item"
    class:active={filesOpen}
    type="button"
    aria-label="Files"
    aria-pressed={filesOpen}
    title={filesOpen ? 'Hide files' : 'Show files'}
    onclick={ontogglefiles}
  >
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M3.5 7.5a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" />
    </svg>
  </button>
  <button class="item" type="button" aria-label="Search" title="Search (coming soon)" disabled>
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="10.5" cy="10.5" r="6" />
      <path d="m15 15 5 5" />
    </svg>
  </button>

  <div class="spacer"></div>

  <button class="item" type="button" aria-label="Settings" title="Settings (coming soon)" disabled>
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M4 7h10M18 7h2M4 17h4M12 17h8" />
      <circle cx="16" cy="7" r="2" />
      <circle cx="10" cy="17" r="2" />
    </svg>
  </button>
</nav>

<style>
  .rail {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-3) 0;
    background: var(--bg-sunken);
    border-right: 1px solid var(--line);
  }
  .brand {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    margin-bottom: var(--space-3);
  }
  .spacer {
    flex: 1;
  }
  .item {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .item:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text);
  }
  .item.active {
    background: var(--accent-soft);
    color: var(--accent);
  }
  .item:disabled {
    cursor: default;
    opacity: 0.45;
  }
  svg {
    width: 20px;
    height: 20px;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.7;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
