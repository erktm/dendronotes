import { describe, expect, it } from 'vitest'
import tokensCSS from '../../styles/tokens.css?raw'
import drawingCSS from '../../styles/drawing.css?raw'
import { COLORS, COLOR_VALUES, styleSheet } from './colors'

// The --draw-* tokens of a block of tokens.css (by the first token's value).
function tokens(selectorStart: string): Map<string, string> {
  const css = tokensCSS.replace(/\/\*[\s\S]*?\*\//g, '')
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    if (!m[1]!.trim().startsWith(selectorStart) || !m[2]!.includes('--draw-')) continue
    return new Map([...m[2]!.matchAll(/--(draw-[a-z-]+):\s*([^;]+);/g)].map((t) => [t[1]!, t[2]!.trim()]))
  }
  throw new Error(`no drawing colors in ${selectorStart}`)
}

describe('drawing colors', () => {
  it('are the same in the app and in saved files', () => {
    const light = tokens(':root')
    const darkSystem = tokens(":root:not([data-theme='light'])")
    const darkChosen = tokens(":root[data-theme='dark']")
    for (const c of COLORS.filter((c) => c !== 'ink')) {
      const v = COLOR_VALUES[c]
      expect([light.get(`draw-${c}`), light.get(`draw-${c}-fill`)], c).toEqual([v.stroke.light, v.fill.light])
      for (const dark of [darkSystem, darkChosen]) {
        expect([dark.get(`draw-${c}`), dark.get(`draw-${c}-fill`)], c).toEqual([v.stroke.dark, v.fill.dark])
      }
    }
  })

  it('all have their classes in the app', () => {
    for (const c of COLORS) {
      for (const cls of [`s-${c}`, `t-${c}`, `f-${c}`]) expect(drawingCSS, cls).toContain(`.dn-drawing .${cls} {`)
    }
  })

  it('switch with the system in saved files', () => {
    const css = styleSheet(['blue', 'ink'])
    expect(css).toContain(`.s-blue{stroke:${COLOR_VALUES.blue.stroke.light}}`)
    expect(css).toContain(`@media (prefers-color-scheme: dark){.s-blue{stroke:${COLOR_VALUES.blue.stroke.dark}}`)
    expect(css).not.toMatch(/url\(|@import|\\|</)
  })
})
