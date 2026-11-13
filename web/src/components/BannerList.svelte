<script lang="ts">
  import Icon from './Icon.svelte'
  import type { ShownBanner } from '../lib/api/soma'
  import { bannerHTML } from '../lib/bannerText'
  import { strings } from '../lib/strings'

  // A stack of banners. The text goes only through bannerHTML (sanitized).
  let {
    banners,
    ondismiss,
    variant = 'bar',
  }: {
    banners: readonly Pick<ShownBanner, 'id' | 'message' | 'kind' | 'dismissible'>[]
    ondismiss?: (id: number) => void
    /** bar: full width above the app; card: rounded, above the sign-in card. */
    variant?: 'bar' | 'card'
  } = $props()
</script>

{#if banners.length}
  <div class="banners {variant}" role="region" aria-label={strings.banners.region}>
    {#each banners as b (b.id)}
      <div class="banner {b.kind}" role={b.kind === 'critical' ? 'alert' : 'status'}>
        <span class="icon" aria-label={strings.banners.kinds[b.kind]}>
          <Icon name={b.kind === 'info' ? 'info' : 'alert'} size={16} />
        </span>
        <p class="text">{@html bannerHTML(b.message)}</p>
        {#if b.dismissible && ondismiss}
          <button type="button" class="close" aria-label={strings.banners.close} title={strings.banners.close} onclick={() => ondismiss(b.id)}>
            <Icon name="close" size={14} />
          </button>
        {/if}
      </div>
    {/each}
  </div>
{/if}

<style>
  .banners {
    display: flex;
    flex-direction: column;
  }
  .banners.card {
    gap: var(--space-2);
    width: min(100%, 560px);
  }
  .banner {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: 8px var(--space-4);
    border-bottom: 1px solid var(--line);
    background: var(--accent-soft);
    color: var(--text);
    font-size: var(--text-sm);
    line-height: 1.45;
  }
  .card .banner {
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow);
  }
  .banner.warning {
    background: var(--warn-soft);
  }
  .banner.warning .icon {
    color: var(--warn-text);
  }
  .banner.critical {
    background: var(--bg-raised);
    box-shadow: inset 4px 0 0 var(--err);
  }
  .card .banner.critical {
    box-shadow: inset 4px 0 0 var(--err), var(--shadow);
  }
  .banner.critical .icon {
    color: var(--err);
  }
  .icon {
    display: grid;
    flex: none;
    place-items: center;
    height: 20px;
    color: var(--accent-strong);
  }
  .text {
    flex: 1;
    min-width: 0;
    margin: 0;
    overflow-wrap: anywhere;
  }
  .text :global(a) {
    color: inherit;
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .text :global(code) {
    padding: 0 3px;
    border-radius: 3px;
    background: var(--code-bg);
    font-size: 0.92em;
  }
  .close {
    display: grid;
    flex: none;
    place-items: center;
    width: 24px;
    height: 24px;
    margin: -2px -6px -2px 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .close:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .close:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
  }
</style>
