// Built-in icons for drawings: simple line pictures drawn for dendronotes on
// a 24 × 24 grid (round caps and joins, no fill), in the style of the app's
// own icons. They are part of dendronotes itself – no third-party set, no
// trademarks. In a drawing an icon is an element ("icon") that names one of
// them, so it takes the stroke color and line width like any shape.

// Their names and the group titles are in strings.ts (drawing.builtin).

export interface BuiltinIcon {
  name: string
  /** Path data on a 24 × 24 grid. */
  d: string
}

export interface IconGroup {
  id: string
  icons: BuiltinIcon[]
}

const circle = (cx: number, cy: number, r: number) => `M${cx} ${cy - r}a${r} ${r} 0 1 1 0 ${2 * r}a${r} ${r} 0 1 1 0 ${-2 * r}z`

export const ICON_GROUPS: IconGroup[] = [
  {
    id: 'project',
    icons: [
      { name: 'task', d: 'M4.5 4.5h15v15h-15zM8 12.5l3 3 5.5-6' },
      { name: 'checklist', d: 'M10 6.5h9.5M10 12h9.5M10 17.5h9.5M4 6.5l1.3 1.3 2.2-2.6M4 12l1.3 1.3 2.2-2.6M4.5 17.5h2.5' },
      { name: 'board', d: 'M3.5 4.5h17v15h-17zM9.2 4.5v15M14.8 4.5v15M5.5 8h2M5.5 11h2M11 8h2M16.5 8h2M16.5 11h2M16.5 14h2' },
      { name: 'timeline', d: 'M4 4v16h16M7 7.5h6M10 11h7M8.5 14.5h5.5M13 18h5' },
      { name: 'milestone', d: 'M12 3.5l4 4-4 4-4-4zM12 11.5v9M9 20.5h6' },
      { name: 'flag', d: 'M5.5 21V4M5.5 4.5h11.5l-2.5 4 2.5 4H5.5' },
      { name: 'calendar', d: 'M4.5 6h15v14h-15zM4.5 10h15M8.5 4v4M15.5 4v4M8 13.5h2M14 13.5h2M8 16.5h2' },
      { name: 'deadline', d: `${circle(12, 12.5, 8)}M12 8v4.5l3 2M9.5 2.5h5` },
      { name: 'person', d: `${circle(12, 8.5, 3.8)}M4.5 20a7.5 7.5 0 0 1 15 0` },
      { name: 'team', d: `${circle(9, 8.5, 3.3)}M2.8 19.5a6.2 6.2 0 0 1 12.4 0M15.5 5.2a3.3 3.3 0 0 1 0 6.1M17.5 14.2a6.2 6.2 0 0 1 3.7 5.3` },
      { name: 'goal', d: `${circle(12, 12, 8.5)}${circle(12, 12, 4.5)}${circle(12, 12, 0.6)}` },
      { name: 'idea', d: 'M9.5 17.5h5M10.5 20.5h3M12 3.5a6 6 0 0 0-3.6 10.8c.7.5 1.1 1.3 1.1 2.1v1.1h5v-1.1c0-.8.4-1.6 1.1-2.1A6 6 0 0 0 12 3.5z' },
      { name: 'document', d: 'M7 3.5h7l4 4v12a1.5 1.5 0 0 1-1.5 1.5h-9.5A1.5 1.5 0 0 1 5.5 19.5V5A1.5 1.5 0 0 1 7 3.5zM13.5 3.5V8h4.5M8.5 12.5h7M8.5 16h5' },
      { name: 'meeting', d: 'M4 5h11a1.5 1.5 0 0 1 1.5 1.5v6a1.5 1.5 0 0 1-1.5 1.5H8.5L5 17v-3H4a1.5 1.5 0 0 1-1.5-1.5v-6A1.5 1.5 0 0 1 4 5zM19 9h.5A1.5 1.5 0 0 1 21 10.5v6a1.5 1.5 0 0 1-1.5 1.5H19v3l-3.5-3H11' },
      { name: 'risk', d: 'M12 9v4M12 16.5v.01M10.3 4.3 3.2 17a2 2 0 0 0 1.7 3h14.2a2 2 0 0 0 1.7-3L13.7 4.3a2 2 0 0 0-3.4 0z' },
      { name: 'approved', d: `${circle(12, 12, 8.5)}M8 12.5l2.8 2.8L16.5 9.5` },
    ],
  },
  {
    id: 'software',
    icons: [
      { name: 'code', d: 'M8.5 7l-5 5 5 5M15.5 7l5 5-5 5M13.5 4.5l-3 15' },
      { name: 'terminal', d: 'M3.5 5h17v14h-17zM7 9.5l3 2.5-3 2.5M12 15h5' },
      { name: 'branch', d: `${circle(6.5, 6, 2)}${circle(6.5, 18, 2)}${circle(17.5, 8, 2)}M6.5 8v8M17.5 10c0 4-11 2.5-11 6` },
      { name: 'merge', d: `${circle(6.5, 6, 2)}${circle(6.5, 18, 2)}${circle(17.5, 18, 2)}M6.5 8v8M17.5 16v-6a3 3 0 0 0-3-3h-3.5M13 5l-2 2 2 2` },
      { name: 'commit', d: `${circle(12, 12, 3.5)}M3 12h5.5M15.5 12H21` },
      { name: 'bug', d: 'M8.5 10a3.5 3.5 0 0 1 7 0v4.5a3.5 3.5 0 0 1-7 0zM12 10v8M4.5 12.5h4M15.5 12.5h4M5 7.5l3.5 2.5M19 7.5l-3.5 2.5M5 18.5l3.5-2.5M19 18.5l-3.5-2.5M10.5 6.8 9 4.5M13.5 6.8l1.5-2.3' },
      { name: 'database', d: 'M12 8.5c4.1 0 7.5-1.2 7.5-2.8S16.1 3 12 3 4.5 4.2 4.5 5.7 7.9 8.5 12 8.5zM4.5 5.7v12.6C4.5 19.8 7.9 21 12 21s7.5-1.2 7.5-2.7V5.7M4.5 12c0 1.5 3.4 2.8 7.5 2.8s7.5-1.3 7.5-2.8' },
      { name: 'api', d: 'M8 4.5c-1.8 0-2.5.9-2.5 2.5v2.5c0 1.2-.7 2.5-2 2.5 1.3 0 2 1.3 2 2.5V17c0 1.6.7 2.5 2.5 2.5M16 4.5c1.8 0 2.5.9 2.5 2.5v2.5c0 1.2.7 2.5 2 2.5-1.3 0-2 1.3-2 2.5V17c0 1.6-.7 2.5-2.5 2.5M9.5 12h.01M12 12h.01M14.5 12h.01' },
      { name: 'settings', d: `${circle(12, 12, 2.8)}${circle(12, 12, 6)}M12 3.5V6M12 18v2.5M3.5 12H6M18 12h2.5M6 6l1.8 1.8M16.2 16.2 18 18M6 18l1.8-1.8M16.2 7.8 18 6` },
      { name: 'package', d: 'M12 3.5l8 4v9l-8 4-8-4v-9zM4 7.5l8 4 8-4M12 11.5v9M8 5.5l8 4' },
      { name: 'container', d: 'M3.5 7.5h17v12h-17zM3.5 7.5l2-3.5h13l2 3.5M7.5 10.5v6M10.5 10.5v6M13.5 10.5v6M16.5 10.5v6' },
      { name: 'test', d: 'M9 3.5h6M10.5 3.5v6L5 19a1 1 0 0 0 .9 1.5h12.2A1 1 0 0 0 19 19l-5.5-9.5v-6M7.2 15h9.6' },
      { name: 'browser', d: 'M3.5 5h17v14h-17zM3.5 9h17M6.5 7h.01M9 7h.01M11.5 7h.01' },
      { name: 'mobile', d: 'M8 3.5h8a1 1 0 0 1 1 1v15a1 1 0 0 1-1 1H8a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1zM11 17.5h2' },
      { name: 'pipeline', d: 'M19 12a7 7 0 1 1-2.05-4.95M19 4.5V8h-3.5M9.5 9.5l5 2.5-5 2.5z' },
      { name: 'release', d: 'M12 3.5c3 2 4.5 5 4.5 9l-2 3h-5l-2-3c0-4 1.5-7 4.5-9zM9.5 15.5 7 18.5l1 2 2.5-1M14.5 15.5l2.5 3-1 2-2.5-1M12 9.5h.01' },
    ],
  },
  {
    id: 'cloud',
    icons: [
      { name: 'cloud', d: 'M7 18.5h10.5a4 4 0 0 0 .5-7.97A5.75 5.75 0 0 0 6.85 9.6 4.5 4.5 0 0 0 7 18.5z' },
      { name: 'cloud-upload', d: 'M8 18.5H7a4.5 4.5 0 0 1-.15-8.9A5.75 5.75 0 0 1 18 10.53a4 4 0 0 1-.5 7.97H16M12 20.5v-7M9.5 15.5 12 13l2.5 2.5' },
      { name: 'cloud-download', d: 'M8 18.5H7a4.5 4.5 0 0 1-.15-8.9A5.75 5.75 0 0 1 18 10.53a4 4 0 0 1-.5 7.97H16M12 13v7.5M9.5 18 12 20.5l2.5-2.5' },
      { name: 'server', d: 'M4 4.5h16v6H4zM4 13.5h16v6H4zM7 7.5h.01M7 16.5h.01M11 7.5h6M11 16.5h6' },
      { name: 'storage', d: 'M4.5 6.5h15l-1.4 12.6a1.5 1.5 0 0 1-1.5 1.4H7.4a1.5 1.5 0 0 1-1.5-1.4zM4.5 6.5C4.5 5 7.9 3.5 12 3.5s7.5 1.5 7.5 3M8 11h8' },
      { name: 'function', d: 'M6.5 20 12 9M9 4h1.5c1 0 1.8.6 2.1 1.5L17.5 20h1.5' },
      { name: 'load-balancer', d: `${circle(12, 5, 2)}${circle(5, 19, 2)}${circle(12, 19, 2)}${circle(19, 19, 2)}M12 7v10M12 9.5l-6.2 7.7M12 9.5l6.2 7.7` },
      { name: 'queue', d: 'M3 8h3.5v8H3zM8.5 8H12v8H8.5zM14 12h7M18.5 9.5 21 12l-2.5 2.5' },
      { name: 'lock', d: 'M7 11V8a5 5 0 0 1 10 0v3M6 11h12a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1zM12 14.5v2' },
      { name: 'key', d: `${circle(7.5, 12, 3.5)}M11 12h9.5M17.5 12v3M20.5 12v2.5` },
      { name: 'globe', d: `${circle(12, 12, 8.5)}M3.5 12h17M12 3.5c2.3 2.3 3.5 5.2 3.5 8.5s-1.2 6.2-3.5 8.5c-2.3-2.3-3.5-5.2-3.5-8.5s1.2-6.2 3.5-8.5z` },
      { name: 'monitoring', d: 'M3.5 19.5h17M6.5 16v-4M10.5 16V8M14.5 16v-6M18.5 16V5' },
      { name: 'region', d: `M12 21s6.5-6.1 6.5-11a6.5 6.5 0 1 0-13 0c0 4.9 6.5 11 6.5 11z${circle(12, 10, 2.2)}` },
      { name: 'cluster', d: `M12 3l7.8 4.5v9L12 21l-7.8-4.5v-9z${circle(12, 12, 2.5)}M12 9.5V3M14.2 13.3l5.6 3.2M9.8 13.3l-5.6 3.2` },
      { name: 'database-cloud', d: 'M10 7c3.3 0 6-1 6-2.2S13.3 2.5 10 2.5 4 3.5 4 4.8 6.7 7 10 7zM4 4.8v9.4c0 1.1 2.4 2 5.5 2.2M16 4.8v4.7M4 9.5c0 1.2 2.7 2.2 6 2.2M13.5 21h6a2.5 2.5 0 0 0 .3-5A3.5 3.5 0 0 0 13 15.4 2.8 2.8 0 0 0 13.5 21z' },
      { name: 'cdn', d: `${circle(12, 12, 3)}${circle(4.5, 5, 1.8)}${circle(19.5, 5, 1.8)}${circle(4.5, 19, 1.8)}${circle(19.5, 19, 1.8)}M6 6.5l3.8 3.4M18 6.5l-3.8 3.4M6 17.5l3.8-3.4M18 17.5l-3.8-3.4` },
    ],
  },
  {
    id: 'network',
    icons: [
      { name: 'router', d: 'M3.5 13.5h17v6h-17zM6.5 16.5h.01M9.5 16.5h.01M8.5 13.5 6.5 7.5M15.5 13.5l2-6M9.5 5.5a4.5 4.5 0 0 1 5 0' },
      { name: 'switch', d: 'M3.5 9.5h17v5h-17zM6.5 12h.01M9.5 12h.01M12.5 12h.01M15.5 12h.01M7 5.5h10M15 3.5l2 2-2 2M17 18.5H7M9 16.5l-2 2 2 2' },
      { name: 'firewall', d: 'M3.5 5h17v14h-17zM3.5 9.7h17M3.5 14.3h17M9.5 5v4.7M15.5 5v4.7M12.5 9.7v4.6M6.5 14.3V19M18 14.3V19' },
      { name: 'laptop', d: 'M5 5.5h14v10H5zM2.5 18.5h19l-1.5-3H4z' },
      { name: 'desktop', d: 'M3.5 4.5h17v11h-17zM9 20h6M12 15.5V20' },
      { name: 'phone', d: 'M8 3.5h8a1 1 0 0 1 1 1v15a1 1 0 0 1-1 1H8a1 1 0 0 1-1-1v-15a1 1 0 0 1 1-1zM11 6h2M12 17.5h.01' },
      { name: 'wifi', d: 'M12 18.5h.01M8.5 15a5 5 0 0 1 7 0M5.5 12a9 9 0 0 1 13 0M2.5 9a13.5 13.5 0 0 1 19 0' },
      { name: 'rack', d: 'M5 3.5h14v17H5zM5 8.5h14M5 13.5h14M8 6h.01M8 11h.01M8 16h.01M11 6h5M11 11h5M11 16h5' },
      { name: 'topology', d: `${circle(12, 5.5, 2.5)}${circle(5, 18.5, 2.5)}${circle(19, 18.5, 2.5)}M12 8v4M12 12l-5.3 4.5M12 12l5.3 4.5` },
      { name: 'plug', d: 'M9 3.5v4M15 3.5v4M7 7.5h10v3.5a5 5 0 0 1-10 0zM12 16v4.5' },
      { name: 'vpn', d: 'M12 3.5 5 6.2v5.3c0 4.3 2.9 7.6 7 9 4.1-1.4 7-4.7 7-9V6.2zM9 12l2.2 2.2L15.5 10' },
      { name: 'antenna', d: 'M12 11v9.5M9 20.5h6M9.2 8.2a4 4 0 0 1 5.6 0M6.5 5.5a7.8 7.8 0 0 1 11 0M12 11h.01' },
      { name: 'mail', d: 'M3.5 6h17v12h-17zM3.5 6.5l8.5 7 8.5-7' },
      { name: 'dns', d: `${circle(12, 12, 8.5)}M3.5 12h17M12 3.5c2.3 2.3 3.5 5.2 3.5 8.5M7.5 16.5h.01M10.5 16.5h6` },
      { name: 'printer', d: 'M7 8V3.5h10V8M7 17.5H4.5a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1h15a1 1 0 0 1 1 1v7.5a1 1 0 0 1-1 1H17M7 14h10v6.5H7z' },
      { name: 'user-device', d: `M4 4.5h11v8H4zM2.5 15.5h14M7.5 12.5v3${circle(18.5, 15, 2.5)}M14.5 21a4 4 0 0 1 8 0` },
    ],
  },
]

const byName = new Map(ICON_GROUPS.flatMap((g) => g.icons).map((i) => [i.name, i]))

/** A built-in icon by name. */
export function builtinIcon(name: string): BuiltinIcon | undefined {
  return byName.get(name)
}
