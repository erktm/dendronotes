<script lang="ts" module>
  // Simple line icons, drawn on a 24×24 grid with round caps.
  const icons = {
    chevron: 'M9 6l6 6-6 6',
    'chevron-down': 'M6 9l6 6 6-6',
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
    info: 'M12 20.5a8.5 8.5 0 1 0 0-17 8.5 8.5 0 0 0 0 17zM12 11v5.5M12 7.8v.01',
    megaphone: 'M4 10v4a1 1 0 0 0 1 1h2l8 4.5V4.5L7 9H5a1 1 0 0 0-1 1zM7 15l1.2 4.5h2.3L9.8 15M18.5 9.5a3.5 3.5 0 0 1 0 5',
    close: 'M6 6l12 12M18 6 6 18',
    search: 'M15 15l5 5M10.5 16.5a6 6 0 1 0 0-12 6 6 0 0 0 0 12z',
    panel: 'M4.5 5h15a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM15 5v14',
    refresh: 'M19 12a7 7 0 1 1-2.05-4.95M19 4.5V8h-3.5',
    upload: 'M12 15.5V4.5M7.5 9 12 4.5 16.5 9M5 19.5h14',
    minus: 'M5 12h14',
    shield: 'M12 3.5 5 6.2v5.3c0 4.3 2.9 7.6 7 9 4.1-1.4 7-4.7 7-9V6.2zM9 12l2.2 2.2L15.5 10',
    user: 'M12 12a3.8 3.8 0 1 0 0-7.6 3.8 3.8 0 0 0 0 7.6zM4.5 20a7.5 7.5 0 0 1 15 0',
    group: 'M9 11.5a3.3 3.3 0 1 0 0-6.6 3.3 3.3 0 0 0 0 6.6zM2.8 19.5a6.2 6.2 0 0 1 12.4 0M15.5 5.2a3.3 3.3 0 0 1 0 6.1M17.5 14.2a6.2 6.2 0 0 1 3.7 5.3',
    fit: 'M4.5 9V5.5a1 1 0 0 1 1-1H9M15 4.5h3.5a1 1 0 0 1 1 1V9M19.5 15v3.5a1 1 0 0 1-1 1H15M9 19.5H5.5a1 1 0 0 1-1-1V15',
    graph: 'M7 7.5a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM17.5 10a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM9 20.5a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM8.5 7l7.3 1.3M7.3 7.5l1.4 9M10.8 17.6l5.4-6.9',
    download: 'M12 4.5v11M7.5 11 12 15.5 16.5 11M5 19.5h14',
    history: 'M4.5 12a7.5 7.5 0 1 0 2.2-5.3M4.5 4.5v3.7h3.7M12 8v4.5l3 2',
    comment: 'M5 5h14a1.5 1.5 0 0 1 1.5 1.5v8.5a1.5 1.5 0 0 1-1.5 1.5h-8.5L6 20v-3.5H5A1.5 1.5 0 0 1 3.5 15V6.5A1.5 1.5 0 0 1 5 5z',
    bell: 'M6 16.5V11a6 6 0 0 1 12 0v5.5l1.5 2h-15zM10 20.5a2 2 0 0 0 4 0',
    check: 'M5 12.5l4.5 4.5L19 7.5',
    restore: 'M9 14 4.5 9.5 9 5M4.5 9.5h9.5a5.5 5.5 0 0 1 0 11H11',
    external: 'M13.5 4.5h6v6M19.5 4.5 11 13M17.5 14v4.5a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1v-11a1 1 0 0 1 1-1H10',
    drawing:
      'M7 3.5h7l4 4v12a1.5 1.5 0 0 1-1.5 1.5h-9.5a1.5 1.5 0 0 1-1.5-1.5v-14.5a1.5 1.5 0 0 1 1.5-1.5zM13.5 3.5v4.5h4.5M8.5 17c1.2-2.6 2.5-2.6 3.5 0s2.3 1 3-1',
    pointer: 'M6 4.5l12 6.3-5.4 1.6-2.4 5.4z',
    move: 'M12 4v16M4 12h16M12 4 9.5 6.5M12 4l2.5 2.5M12 20l-2.5-2.5M12 20l2.5-2.5M4 12l2.5-2.5M4 12l2.5 2.5M20 12l-2.5-2.5M20 12l-2.5 2.5',
    square: 'M5.5 5.5h13v13h-13z',
    circle: 'M12 19a7 7 0 1 0 0-14 7 7 0 0 0 0 14z',
    diamond: 'M12 3.5 20.5 12 12 20.5 3.5 12z',
    line: 'M5 19 19 5',
    arrow: 'M5 19 19 5M11 5h8v8',
    elbow: 'M4.5 18.5h7.5v-13h7.5',
    text: 'M6 7V5h12v2M12 5v14M9.5 19h5',
    image: 'M5 5h14a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1zM4 16l4.5-4.5 3.5 3.5 2.5-2.5 5.5 5.5M15.5 9.5v.01',
    undo: 'M9 14 4.5 9.5 9 5M4.5 9.5H15a5 5 0 0 1 0 10h-3',
    redo: 'M15 14l4.5-4.5L15 5M19.5 9.5H9a5 5 0 0 0 0 10h3',
    'zoom-in': 'M15 15l5 5M10.5 16.5a6 6 0 1 0 0-12 6 6 0 0 0 0 12zM8 10.5h5M10.5 8v5',
    'zoom-out': 'M15 15l5 5M10.5 16.5a6 6 0 1 0 0-12 6 6 0 0 0 0 12zM8 10.5h5',
    expand: 'M14 4.5h5.5V10M19.5 4.5l-6 6M10 19.5H4.5V14M4.5 19.5l6-6',
    copy: 'M9 9h10v10H9zM15 9V5H5v10h4',
    'to-front': 'M12 20V8M7.5 12.5 12 8l4.5 4.5M5 4.5h14',
    'to-back': 'M12 4v12M7.5 11.5 12 16l4.5-4.5M5 19.5h14',
    forward: 'M12 19V9M7.5 13.5 12 9l4.5 4.5',
    backward: 'M12 5v10M7.5 10.5 12 15l4.5-4.5',
    grid: 'M4.5 4.5h15v15h-15zM9.5 4.5v15M14.5 4.5v15M4.5 9.5h15M4.5 14.5h15',
    sketch: 'M4 15c2-6 4-6 5 0s3 6 5 0 3-6 6-2',
  } as const

  export type IconName = keyof typeof icons

  /** The path of an icon, for SVG built outside Svelte. */
  export const iconPath = (name: IconName): string => icons[name]
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
