<script lang="ts">
  let { size = 24 }: { size?: number } = $props()
</script>

<!--
  A dendrite reduced to a few strokes: a stem that branches twice, reaching
  out for synapses. Drawn in the accent color of the palette.
-->
<svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
  <path d="M16 26V12.6L8.6 6.2M16 12.6l7.4-6.4M16 21l-5.6-4.8M16 17.4l5.4-4.4" />
</svg>

<style>
  path {
    fill: none;
    stroke: var(--accent);
    stroke-width: 2.3;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
