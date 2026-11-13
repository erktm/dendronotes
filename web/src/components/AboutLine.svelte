<script lang="ts">
  // The about line at the very bottom: version (when signed in), license and
  // where the source code lives. Nothing else.
  import { brand, strings } from '../lib/strings'

  let { version = '' }: { version?: string } = $props()
  const t = strings.about
</script>

<footer class="about" aria-label={t.label}>
  <span>{version ? `${brand.name} ${version}` : brand.name}</span>
  <span aria-hidden="true">·</span>
  <span>{t.license}</span>
  <span aria-hidden="true">·</span>
  <a href={brand.repository} target="_blank" rel="noopener noreferrer">{t.source}</a>
</footer>

<style>
  .about {
    display: flex;
    flex: none;
    flex-wrap: wrap;
    align-items: center;
    justify-content: center;
    gap: 6px;
    padding: 4px var(--space-3);
    color: var(--text-faint);
    font-size: var(--text-xs);
    line-height: 1.4;
  }
  a {
    color: var(--text-muted);
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  a:hover {
    color: var(--text);
  }
</style>
