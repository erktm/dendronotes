<script lang="ts">
  import type { Snippet } from 'svelte'
  import Logo from './Logo.svelte'

  let {
    title = 'Nothing open yet',
    text = 'Pick a note on the left or create a new one. Your notes stay plain .md files in your vault.',
    children,
  }: { title?: string; text?: string; children?: Snippet } = $props()
</script>

<section class="empty">
  <div class="mark"><Logo size={64} /></div>
  <h1>{title}</h1>
  <p>{text}</p>
  {@render children?.()}
</section>

<style>
  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-3);
    height: 100%;
    padding: var(--space-8) var(--space-6);
    text-align: center;
  }
  .mark {
    display: grid;
    place-items: center;
    width: 104px;
    height: 104px;
    margin-bottom: var(--space-2);
    border-radius: 50%;
    background: var(--accent-soft);
  }
  h1 {
    font-size: var(--text-xl);
    font-weight: 600;
    letter-spacing: -0.015em;
  }
  p {
    max-width: 44ch;
    color: var(--text-muted);
  }
</style>
