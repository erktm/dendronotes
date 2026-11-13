import { describe, expect, it } from 'vitest'
import { bindTargetAt, elementsIn, hitTest, resizeBox } from './hit'
import { approxMeasure } from './render'
import { type Element, type ShapeEl, add, emptyScene } from './scene'

function shape(id: string, x: number, y: number, extra: Partial<ShapeEl> = {}): ShapeEl {
  return {
    id, type: 'rect', x, y, w: 100, h: 100, stroke: 'ink', fill: 'none', width: 2, dash: 'solid', seed: 1, rounded: false, flip: false,
    text: '', font: 'sans', size: 20, align: 'center', ...extra,
  }
}

const scene = (...els: Element[]) => add(emptyScene(), ...els)

describe('hits', () => {
  it('find the topmost element', () => {
    const s = scene(shape('a', 0, 0, { fill: 'blue' }), shape('b', 50, 50, { fill: 'red' }))
    expect(hitTest(s, [60, 60], 4, approxMeasure)?.id).toBe('b')
    expect(hitTest(s, [10, 10], 4, approxMeasure)?.id).toBe('a')
    expect(hitTest(s, [300, 300], 4, approxMeasure)).toBeNull()
  })

  it('pick empty shapes at their outline, so what lies inside stays reachable', () => {
    const s = scene(shape('frame', 0, 0))
    expect(hitTest(s, [50, 50], 4, approxMeasure)).toBeNull()
    expect(hitTest(s, [1, 50], 4, approxMeasure)?.id).toBe('frame')
    const labelled = scene(shape('box', 0, 0, { text: 'hi' }))
    expect(hitTest(labelled, [50, 50], 4, approxMeasure)?.id).toBe('box')
  })

  it('find what a line can stick to', () => {
    const s = scene(shape('a', 0, 0), { ...shape('e', 200, 0), type: 'ellipse' })
    expect(bindTargetAt(s, [50, 50], 8, new Set())?.id).toBe('a')
    expect(bindTargetAt(s, [104, 50], 8, new Set())?.id).toBe('a')
    expect(bindTargetAt(s, [202, 2], 8, new Set())).toBeNull() // outside the ellipse's corner
    expect(bindTargetAt(s, [50, 50], 8, new Set(['a']))).toBeNull()
  })

  it('select what lies completely inside a frame', () => {
    const s = scene(shape('a', 0, 0), shape('b', 150, 0))
    expect(elementsIn(s, { x: -10, y: -10, w: 200, h: 200 })).toEqual(['a'])
  })
})

describe('resizing', () => {
  const b = { x: 0, y: 0, w: 100, h: 50 }
  it('moves only the dragged sides', () => {
    expect(resizeBox(b, 'se', [200, 80], false)).toEqual({ x: 0, y: 0, w: 200, h: 80 })
    expect(resizeBox(b, 'w', [-50, 999], false)).toEqual({ x: -50, y: 0, w: 150, h: 50 })
    expect(resizeBox(b, 'n', [999, 120], false)).toEqual({ x: 0, y: 46, w: 100, h: 4 })
  })

  it('keeps the proportions when asked', () => {
    const r = resizeBox(b, 'se', [300, 60], true)
    expect(r.w / r.h).toBeCloseTo(2)
    expect([r.x, r.y]).toEqual([0, 0])
    const e = resizeBox(b, 'e', [200, 0], true)
    expect(e).toEqual({ x: 0, y: -25, w: 200, h: 100 })
  })
})
