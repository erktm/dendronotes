// The colors of drawings: fixed slots instead of free colors, so a drawing
// looks right in light and dark and in every palette. In the app the slots
// are the tokens --draw-* (styles/tokens.css; ink is the palette's text); a
// saved file carries these values in its own style sheet, so other viewers
// show it the same way. colors.test.ts checks that both agree.

export const COLORS = ['ink', 'gray', 'red', 'orange', 'yellow', 'green', 'teal', 'blue', 'violet', 'pink'] as const
export type Color = (typeof COLORS)[number]
export type Fill = Color | 'none'

interface Shades {
  /** Lines and text. */
  stroke: { light: string; dark: string }
  /** A pale tint for areas. */
  fill: { light: string; dark: string }
}

export const COLOR_VALUES: Record<Color, Shades> = {
  ink: { stroke: { light: '#1c1c1b', dark: '#ededed' }, fill: { light: '#ebebe9', dark: '#2a2a2a' } },
  gray: { stroke: { light: '#6b7280', dark: '#a1a1aa' }, fill: { light: '#e5e7eb', dark: '#3f3f46' } },
  red: { stroke: { light: '#dc2626', dark: '#f87171' }, fill: { light: '#fee2e2', dark: '#5b1f1f' } },
  orange: { stroke: { light: '#c2410c', dark: '#fb923c' }, fill: { light: '#ffedd5', dark: '#5a2d12' } },
  yellow: { stroke: { light: '#a16207', dark: '#facc15' }, fill: { light: '#fef3c7', dark: '#4d3d0c' } },
  green: { stroke: { light: '#15803d', dark: '#4ade80' }, fill: { light: '#dcfce7', dark: '#163f24' } },
  teal: { stroke: { light: '#0f766e', dark: '#2dd4bf' }, fill: { light: '#ccfbf1', dark: '#11403b' } },
  blue: { stroke: { light: '#2563eb', dark: '#60a5fa' }, fill: { light: '#dbeafe', dark: '#1e3a5f' } },
  violet: { stroke: { light: '#7c3aed', dark: '#a78bfa' }, fill: { light: '#ede9fe', dark: '#362a5e' } },
  pink: { stroke: { light: '#db2777', dark: '#f472b6' }, fill: { light: '#fce7f3', dark: '#521d38' } },
}

/** The knock-out behind labels on lines (the page color). */
export const PAPER = { light: '#ffffff', dark: '#1e1e1e' }

export function isColor(v: unknown): v is Color {
  return typeof v === 'string' && (COLORS as readonly string[]).includes(v)
}

/** Class names: s- strokes in a color, f- fills with its tint, t- fills with the color itself. */
export const strokeClass = (c: Color) => `s-${c}`
export const tintClass = (c: Color) => `f-${c}`
export const solidClass = (c: Color) => `t-${c}`
export const PAPER_CLASS = 'paper'

/** Light or dark only (an exported picture), or both, switching with the system (a file). */
export type Scheme = 'light' | 'dark' | 'both'

/** The style sheet of a saved drawing, for the colors it uses. */
export function styleSheet(used: Iterable<Color>, scheme: Scheme = 'both'): string {
  const colors = [...new Set(used)].sort()
  const rules = (mode: 'light' | 'dark') =>
    colors
      .map((c) => {
        const v = COLOR_VALUES[c]
        return `.s-${c}{stroke:${v.stroke[mode]}}.t-${c}{fill:${v.stroke[mode]}}.f-${c}{fill:${v.fill[mode]}}`
      })
      .join('') + `.${PAPER_CLASS}{fill:${PAPER[mode]}}`
  if (scheme !== 'both') return rules(scheme)
  return `${rules('light')}@media (prefers-color-scheme: dark){${rules('dark')}}`
}
