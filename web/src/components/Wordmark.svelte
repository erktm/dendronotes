<script lang="ts">
  import { brand } from '../lib/strings'

  let { size = 'md' }: { size?: 'md' | 'lg' } = $props()
</script>

<!-- Always lowercase, by content, not by CSS. -->
<span class="wordmark {size}"><span class="accent">{brand.wordmark.accent}</span>{brand.wordmark.rest}</span>

<style>
  .wordmark {
    color: var(--text);
    font-weight: 500;
    letter-spacing: -0.02em;
    white-space: nowrap;
  }
  .md {
    font-size: var(--text-lg);
  }
  .lg {
    font-size: 1.5rem;
  }
  .accent {
    color: var(--accent);
    font-weight: 650;
  }
</style>
