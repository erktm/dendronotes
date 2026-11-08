import { describe, expect, it } from 'vitest'
import tokensCSS from '../styles/tokens.css?raw'
import { DEFAULT_PALETTE, PALETTES, type Palette } from './palettes'

// Reads the palette blocks of tokens.css: for each palette its light block,
// its dark block for the system setting and its dark block for the choice.
const css = tokensCSS.replace(/\/\*[\s\S]*?\*\//g, '')

type Block = { palette: Palette; mode: 'light' | 'dark-system' | 'dark-chosen'; selector: string; tokens: Map<string, string> }

function blocks(): Block[] {
  const out: Block[] = []
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const selector = m[1]!.replace(/@media[^{]*$/, '').trim()
    const body = m[2]!
    if (!/--bg:/.test(body)) continue // sizes, fonts, peers
    const palette = /\[data-palette='([a-z]+)'\]/.exec(selector)?.[1] as Palette | undefined
    if (!palette) throw new Error(`palette block without [data-palette]: ${selector}`)
    const mode = selector.includes("data-theme='dark'")
      ? 'dark-chosen'
      : selector.includes(":not([data-theme='light'])")
        ? 'dark-system'
        : 'light'
    const tokens = new Map([...body.matchAll(/--([a-z0-9-]+):\s*([^;]+);/g)].map((t) => [t[1]!, t[2]!.trim()]))
    out.push({ palette, mode, selector, tokens })
  }
  return out
}

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
  const f = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
  return 0.2126 * f(r!) + 0.7152 * f(g!) + 0.0722 * f(b!)
}

function contrast(a: string, b: string): number {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p)
  return (x! + 0.05) / (y! + 0.05)
}

describe('color palettes', () => {
  const all = blocks()

  it('defines every palette in three blocks with the same tokens', () => {
    const tokens = [...all[0]!.tokens.keys()].sort()
    expect(tokens).toContain('accent-2')
    for (const p of PALETTES) {
      const own = all.filter((b) => b.palette === p)
      expect(own.map((b) => b.mode).sort(), p).toEqual(['dark-chosen', 'dark-system', 'light'])
      for (const b of own) expect([...b.tokens.keys()].sort(), `${p} ${b.mode}`).toEqual(tokens)
      const [system, chosen] = ['dark-system', 'dark-chosen'].map((m) => own.find((b) => b.mode === m)!)
      expect([...system!.tokens], `${p} dark blocks agree`).toEqual([...chosen!.tokens])
    }
    expect(all.length).toBe(PALETTES.length * 3)
  })

  it('puts the default palette on :root', () => {
    const onRoot = all.filter((b) => /^:root(,|:not\(\[data-theme='light'\]\),|\[data-theme='dark'\],)/.test(b.selector))
    expect(onRoot.map((b) => b.palette)).toEqual([DEFAULT_PALETTE, DEFAULT_PALETTE, DEFAULT_PALETTE])
  })

  it('keeps text readable (WCAG AA)', () => {
    for (const b of all) {
      const t = (name: string) => b.tokens.get(name)!
      for (const fg of ['text', 'text-muted', 'text-faint', 'accent', 'accent-strong', 'accent-2', 'ok', 'warn', 'err']) {
        for (const bg of ['bg', 'bg-raised']) {
          expect(contrast(t(fg), t(bg)), `${b.palette} ${b.mode}: ${fg} on ${bg}`).toBeGreaterThanOrEqual(4.5)
        }
      }
      expect(contrast(t('on-accent'), t('accent')), `${b.palette} ${b.mode}: on-accent`).toBeGreaterThanOrEqual(4.5)
      expect(contrast(t('text'), t('accent-soft')), `${b.palette} ${b.mode}: text on accent-soft`).toBeGreaterThanOrEqual(4.5)
    }
  })
})
