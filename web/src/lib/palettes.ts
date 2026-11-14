// The color palettes defined in styles/tokens.css, in the order the settings
// show them. Names and descriptions live in strings.ts.

export const PALETTES = ['cortex', 'sage', 'golgi', 'nissl', 'eosin', 'myelin', 'confocal', 'brainbow', 'notebook', 'epaper'] as const
export type Palette = (typeof PALETTES)[number]

/** The palette when none is chosen; tokens.css also puts it on :root. */
export const DEFAULT_PALETTE: Palette = 'cortex'

/** Browser and app window color (meta theme-color) of each palette. */
export const THEME_COLORS: Record<Palette, { light: string; dark: string }> = {
  cortex: { light: '#3d3d3a', dark: '#121212' },
  sage: { light: '#2f7a4b', dark: '#0e1411' },
  golgi: { light: '#8a5714', dark: '#13110d' },
  nissl: { light: '#6a4aa8', dark: '#121019' },
  eosin: { light: '#a3305f', dark: '#150e12' },
  myelin: { light: '#2f4fa2', dark: '#0d1020' },
  confocal: { light: '#0b7282', dark: '#0a0d13' },
  brainbow: { light: '#b23f1a', dark: '#121213' },
  notebook: { light: '#26211a', dark: '#18150f' },
  epaper: { light: '#151515', dark: '#101010' },
}

/** The designs that draw more than colors: hand-drawn lines and neurons (styles/sketch.css). */
export const SKETCHED: ReadonlySet<Palette> = new Set(['notebook', 'epaper'])
