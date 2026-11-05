<script lang="ts">
  import Logo from './Logo.svelte'
  import { brand, strings } from '../lib/strings'

  let { filesOpen, ontogglefiles }: { filesOpen: boolean; ontogglefiles: () => void } = $props()
</script>

<nav class="rail" aria-label={strings.nav.main}>
  <div class="brand" title={brand.name}><Logo size={28} /></div>

  <button
    class="item"
    class:active={filesOpen}
    type="button"
    aria-label={strings.nav.files}
    aria-pressed={filesOpen}
    title={filesOpen ? strings.nav.hideFiles : strings.nav.showFiles}
    onclick={ontogglefiles}
  >
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <path d="M3.5 7.5a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" />
    </svg>
  </button>
  <button class="item" type="button" aria-label={strings.nav.search} title={strings.nav.searchSoon} disabled>
    <svg viewBox="0 0 24 24" aria-hidden="true">
      <circle cx="10.5" cy="10.5" r="6" />
      <path d="m15 15 5 5" />
    </svg>
  </button>

  <div class="spacer"></div>

  <button class="item" type="button" aria-label={strings.nav.settings} title={strings.nav.settingsSoon} disabled>
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
