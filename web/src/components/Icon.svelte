<script lang="ts" module>
  // Simple line icons, drawn on a 24×24 grid with round caps.
  const icons = {
    chevron: 'M9 6l6 6-6 6',
    folder: 'M3.5 7.5a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z',
    'folder-open':
      'M3.5 17.5v-10a2 2 0 0 1 2-2h4l2 2h6a2 2 0 0 1 2 2v1M3.5 17.5l2.4-6.3a1.5 1.5 0 0 1 1.4-1h12.2a1 1 0 0 1 .9 1.4l-2.3 5.9a1.5 1.5 0 0 1-1.4 1H5a1.5 1.5 0 0 1-1.5-1z',
    note: 'M7 3.5h7l4 4v12a1.5 1.5 0 0 1-1.5 1.5h-9.5a1.5 1.5 0 0 1-1.5-1.5v-14.5a1.5 1.5 0 0 1 1.5-1.5zM13.5 3.5v4.5h4.5M9 12.5h6M9 16h4',
    file: 'M7 3.5h7l4 4v12a1.5 1.5 0 0 1-1.5 1.5h-9.5a1.5 1.5 0 0 1-1.5-1.5v-14.5a1.5 1.5 0 0 1 1.5-1.5zM13.5 3.5v4.5h4.5',
    plus: 'M12 5v14M5 12h14',
    'folder-plus':
      'M3.5 7.5a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2zM12 11v5M9.5 13.5h5',
    pencil: 'M14.5 5.5l4 4M4 20l1-4.5L15.8 4.7a1.5 1.5 0 0 1 2.1 0l1.4 1.4a1.5 1.5 0 0 1 0 2.1L8.5 19z',
    trash: 'M4.5 7h15M9.5 7V5a1 1 0 0 1 1-1h3a1 1 0 0 1 1 1v2M6.5 7l1 12.5a1.5 1.5 0 0 0 1.5 1.5h6a1.5 1.5 0 0 0 1.5-1.5l1-12.5M10 11v6M14 11v6',
    logout: 'M14 4.5h3.5a1.5 1.5 0 0 1 1.5 1.5v12a1.5 1.5 0 0 1-1.5 1.5H14M9 8l-4 4 4 4M5 12h10',
    lock: 'M7 11V8a5 5 0 0 1 10 0v3M6 11h12a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1z',
    alert: 'M12 9v4M12 16.5v.01M10.3 4.3 3.2 17a2 2 0 0 0 1.7 3h14.2a2 2 0 0 0 1.7-3L13.7 4.3a2 2 0 0 0-3.4 0z',
    close: 'M6 6l12 12M18 6 6 18',
    search: 'M15 15l5 5M10.5 16.5a6 6 0 1 0 0-12 6 6 0 0 0 0 12z',
    panel: 'M4.5 5h15a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM15 5v14',
    refresh: 'M19 12a7 7 0 1 1-2.05-4.95M19 4.5V8h-3.5',
  } as const

  export type IconName = keyof typeof icons
</script>

<script lang="ts">
  let { name, size = 18 }: { name: IconName; size?: number } = $props()
</script>

<svg class="icon" width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
  <path d={icons[name]} />
</svg>

<style>
  .icon {
    flex: none;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.7;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
