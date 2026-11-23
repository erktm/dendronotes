<script lang="ts">
  // The picture of an icon in a library, filling its box. An icon drawn in
  // one dark color (mono) shows in the text color instead, so it stays
  // visible on a dark background; drawings color it the same way.
  import { cssVars } from '../lib/actions'

  let { src, mono = false }: { src: string; mono?: boolean } = $props()
</script>

{#if mono}
  <span class="picture mono" use:cssVars={{ '--picture': `url("${src}")` }}></span>
{:else}
  <img class="picture" {src} alt="" loading="lazy" />
{/if}

<style>
  .picture {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: contain;
  }
  .mono {
    background: var(--text);
    mask: var(--picture) center / contain no-repeat;
  }
</style>
