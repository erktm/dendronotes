import { describe, expect, it } from 'vitest'
import { ICON_GROUPS, builtinIcon } from './icons'
import { strings } from '../strings'
import { sanitizeScene } from './scene'

describe('built-in icons', () => {
  const all = ICON_GROUPS.flatMap((g) => g.icons)

  it('have unique names and a name and group title to show', () => {
    expect(new Set(all.map((i) => i.name)).size).toBe(all.length)
    for (const i of all) expect(strings.drawing.builtin.names[i.name], i.name).toBeTruthy()
    for (const g of ICON_GROUPS) expect(strings.drawing.builtin.groups[g.id], g.id).toBeTruthy()
    expect(builtinIcon('database')?.name).toBe('database')
  })

  it('are plain path data', () => {
    for (const i of all) expect(i.d, i.name).toMatch(/^M[MmLlHhVvCcSsQqAaZz0-9.,\s-]+$/)
  })

  it('come only from dendronotes, never from a file', () => {
    const s = sanitizeScene({
      version: 1,
      elements: [
        { id: 'a', type: 'icon', icon: 'database', x: 0, y: 0, w: 48, h: 48 },
        { id: 'b', type: 'icon', icon: 'M0 0L10 10', x: 0, y: 0, w: 48, h: 48 },
        { id: 'c', type: 'icon', x: 0, y: 0, w: 48, h: 48 },
      ],
    })!
    expect(s.elements.map((e) => e.id)).toEqual(['a'])
  })
})
