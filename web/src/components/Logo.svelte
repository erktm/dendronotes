<script lang="ts">
  let { size = 24 }: { size?: number } = $props()
</script>

<!-- A leaf whose veins double as lines of a note. -->
<svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
  <path class="leaf" d="M7 25C5.5 14 12.5 5.5 27 5c.5 14.5-7 21.5-20 20Z" />
  <path class="stem" d="M7 25 3.5 28.5" />
  <path class="veins" d="M8.5 23.5 21 11M12 20h5M12 20v-5M15.5 16.5h4.5M15.5 16.5v-4.5" />
</svg>

<style>
  .leaf {
    fill: var(--accent);
  }
  .stem {
    fill: none;
    stroke: var(--accent-strong);
    stroke-width: 2;
    stroke-linecap: round;
  }
  .veins {
    fill: none;
    stroke: var(--bg);
    stroke-width: 1.6;
    stroke-linecap: round;
  }
</style>
