<script lang="ts">
  let { size = 24 }: { size?: number } = $props()
</script>

<!--
  A tree whose crown is a neuron: dendrites branch out from the soma (the
  cell body in the middle), ending in synapse tips; the trunk grows down
  from the soma and spreads into small roots.
-->
<svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
  <path
    class="dendrites"
    d="M13.6 11.6 9.5 8 8 4.5M9.5 8 5.5 7.5M18.4 11.6 22.5 8 24 4.5M22.5 8l4 -.5M16 10V5.6M16 5.6 13.8 3M16 5.6 18.2 3M12.8 14.6 8 15.8 4.6 13.6M8 15.8 5.8 18.8M19.2 14.6 24 15.8 27.4 13.6M24 15.8l2.2 3"
  />
  <g class="tips">
    <circle cx="8" cy="4.5" r="1.3" />
    <circle cx="24" cy="4.5" r="1.3" />
    <circle cx="13.8" cy="3" r="1.3" />
    <circle cx="18.2" cy="3" r="1.3" />
    <circle cx="4.6" cy="13.6" r="1.3" />
    <circle cx="27.4" cy="13.6" r="1.3" />
  </g>
  <path class="trunk" d="M14.7 16.4 15.1 28.4Q16 29.3 16.9 28.4L17.3 16.4Z" />
  <path class="roots" d="M15.3 27.4 12.6 29.4M16.7 27.4 19.4 29.4" />
  <circle class="soma" cx="16" cy="13.5" r="3.7" />
  <circle class="nucleus" cx="16" cy="13.5" r="1.35" />
</svg>

<style>
  .dendrites {
    fill: none;
    stroke: var(--accent);
    stroke-width: 1.7;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  .tips,
  .soma {
    fill: var(--accent);
  }
  .trunk {
    fill: var(--accent-strong);
  }
  .roots {
    fill: none;
    stroke: var(--accent-strong);
    stroke-width: 1.4;
    stroke-linecap: round;
  }
  .nucleus {
    fill: var(--bg);
  }
</style>
