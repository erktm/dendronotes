import { describe, expect, it } from 'vitest'
import { BIND_GAP, type LinearEl, SHAPE_TYPES, type Scene, type ShapeEl, add, copies, emptyScene, find, move, outlineOf, remove, reorder, route, sanitizeScene } from './scene'
import { insideOutline, outlineDist } from './geometry'
import { random, sketchEllipse, sketchLines } from './sketch'

function rect(id: string, x: number, y: number, w = 100, h = 60): ShapeEl {
  return { id, type: 'rect', x, y, w, h, stroke: 'ink', fill: 'none', width: 2, dash: 'solid', seed: 1, rounded: false, flip: false, text: '', font: 'sans', size: 20, align: 'center' }
}

function arrow(id: string, from: string | null, to: string | null, elbow = false): LinearEl {
  return {
    id, type: 'arrow', points: [[0, 0], [10, 10]], elbow, start: from ? { id: from } : null, end: to ? { id: to } : null,
    startHead: 'none', endHead: 'arrow', stroke: 'ink', width: 2, dash: 'solid', seed: 2, text: '', font: 'sans', size: 20, align: 'center',
  }
}

const scene = (...els: Parameters<typeof add>[1][]): Scene => add(emptyScene(), ...els)
// Ends keep the gap along the line, which is a little less across the outline.
const near = (d: number) => d > BIND_GAP / 2 && d <= BIND_GAP + 0.01
const line = (s: Scene, id: string) => find(s, id) as LinearEl
const shape = (s: Scene, id: string) => find(s, id) as ShapeEl

describe('lines between shapes', () => {
  it('end on the outlines, a gap away', () => {
    const s = scene(rect('a', 0, 0), rect('b', 300, 0), arrow('l', 'a', 'b'))
    const [p, q] = line(s, 'l').points
    expect(p).toEqual([100 + BIND_GAP, 30])
    expect(q).toEqual([300 - BIND_GAP, 30])
  })

  it('follow a shape that moves', () => {
    let s = scene(rect('a', 0, 0), rect('b', 300, 0), arrow('l', 'a', 'b'))
    s = move(s, new Set(['b']), 0, 200)
    const [p, q] = line(s, 'l').points
    expect(near(outlineDist(p, shape(s, 'a'), 'rect'))).toBe(true)
    expect(near(outlineDist(q, shape(s, 'b'), 'rect'))).toBe(true)
    expect(q[1]).toBeGreaterThan(150)
  })

  it('stick to ellipses and diamonds', () => {
    const e = { ...rect('e', 300, 0), type: 'ellipse' as const }
    const d = { ...rect('d', 0, 200), type: 'diamond' as const }
    const s = scene(e, d, arrow('l', 'd', 'e'))
    const [p, q] = line(s, 'l').points
    expect(near(outlineDist(p, d, 'diamond'))).toBe(true)
    expect(near(outlineDist(q, e, 'ellipse'))).toBe(true)
  })

  it('stick to the outline of every shape, from every side', () => {
    for (const type of SHAPE_TYPES) {
      for (const flip of [false, true]) {
        const target = { ...rect('t', 0, 0, 140, 100), type, flip: flip && (type === 'triangle' || type === 'parallelogram') }
        const o = outlineOf(target)
        expect(insideOutline([70, 50], target, o), `${o} contains its middle`).toBe(true)
        const corners: [number, number][] = [[1, 1], [139, 1], [139, 99], [1, 99]]
        if (o !== 'rect') expect(corners.some((c) => !insideOutline(c, target, o)), `${o} leaves a corner free`).toBe(true)
        for (let i = 0; i < 12; i++) {
          const a = (i / 12) * Math.PI * 2
          const from = rect('f', 70 + Math.cos(a) * 400 - 10, 50 + Math.sin(a) * 400 - 10, 20, 20)
          const s = scene(target, from, arrow('l', 'f', 't'))
          const q = line(s, 'l').points[1]
          // The gap is kept along the line, so across slanted sides it is less.
          const d = outlineDist(q, target, o)
          expect(d > 1 && d <= BIND_GAP + 0.01, `${o} at ${i * 30}°: ${d}`).toBe(true)
          expect(insideOutline(q, target, o), `${o} at ${i * 30}° outside`).toBe(false)
        }
      }
    }
  })

  it('come loose when moved on their own, but not when moved along', () => {
    let s = scene(rect('a', 0, 0), rect('b', 300, 0), arrow('l', 'a', 'b'))
    const both = move(s, new Set(['a', 'b', 'l']), 50, 50)
    expect(line(both, 'l').start).toEqual({ id: 'a' })
    s = move(s, new Set(['l']), 0, 100)
    expect(line(s, 'l').start).toBeNull()
    expect(line(s, 'l').end).toBeNull()
  })

  it('come loose when their shape is removed', () => {
    const s = remove(scene(rect('a', 0, 0), rect('b', 300, 0), arrow('l', 'a', 'b')), new Set(['b']))
    expect(line(s, 'l').end).toBeNull()
    expect(line(s, 'l').start).toEqual({ id: 'a' })
  })

  it('route elbow lines at right angles from the facing sides', () => {
    const s = scene(rect('a', 0, 0), rect('b', 400, 100), arrow('l', 'a', 'b', true))
    const pts = route(line(s, 'l'), s)
    expect(pts).toHaveLength(4)
    expect(pts[0]).toEqual([100 + BIND_GAP, 30])
    expect(pts.at(-1)).toEqual([400 - BIND_GAP, 130])
    for (let i = 1; i < pts.length; i++) {
      const [a, b] = [pts[i - 1]!, pts[i]!]
      expect(a[0] === b[0] || a[1] === b[1]).toBe(true)
    }
  })
})

describe('editing', () => {
  it('copies keep bindings among the copies only', () => {
    const s = scene(rect('a', 0, 0), rect('b', 300, 0), arrow('l', 'a', 'b'))
    const [ca, cl] = copies([find(s, 'a')!, find(s, 'l')!], 20)
    expect(ca!.id).not.toBe('a')
    expect((cl as LinearEl).start).toEqual({ id: ca!.id })
    expect((cl as LinearEl).end).toBeNull()
  })

  it('reorders', () => {
    const s = scene(rect('a', 0, 0), rect('b', 0, 0), rect('c', 0, 0))
    const ids = (x: Scene) => x.elements.map((e) => e.id).join('')
    expect(ids(reorder(s, new Set(['a']), 'front'))).toBe('bca')
    expect(ids(reorder(s, new Set(['c']), 'back'))).toBe('cab')
    expect(ids(reorder(s, new Set(['a']), 'forward'))).toBe('bac')
    expect(ids(reorder(s, new Set(['c']), 'backward'))).toBe('acb')
  })
})

describe('untrusted scenes', () => {
  it('keep only what is known and in range', () => {
    const s = sanitizeScene({
      version: 1,
      sketchy: 'yes',
      elements: [
        { id: 'a', type: 'rect', x: 1e12, y: 'x', w: -5, h: 10, stroke: 'javascript:alert(1)', fill: 'url(#x)', text: 'a\u0000b', onclick: 'x' },
        { id: 'a', type: 'rect' },
        { id: 'bad id!', type: 'rect' },
        { id: 's', type: 'script' },
        { id: 'i', type: 'image', file: 'missing' },
        { id: 'l', type: 'arrow', points: [[0, 0], [5, 5]], start: { id: 'gone' }, end: { id: 'a' }, endHead: 'spear' },
        { id: 'p', type: 'pen', points: [[0, 0], 'x', [1, NaN]] },
      ],
    })!
    expect(s.sketchy).toBe(false)
    expect(s.elements.map((e) => e.id)).toEqual(['a', 'l', 'p'])
    const a = s.elements[0] as ShapeEl
    expect([a.x, a.y, a.w, a.stroke, a.fill, a.text]).toEqual([1e6, 0, 0, 'ink', 'none', 'ab'])
    expect('onclick' in a).toBe(false)
    const l = s.elements[1] as LinearEl
    expect([l.start, l.end, l.endHead]).toEqual([null, { id: 'a' }, 'arrow'])
    expect((s.elements[2] as { points: unknown }).points).toEqual([[0, 0], [1, 0]])
  })

  it('turn over only triangles and parallelograms', () => {
    const s = sanitizeScene({
      version: 1,
      elements: ['triangle', 'parallelogram', 'hexagon', 'cloud'].map((type) => ({ id: type, type, x: 0, y: 0, w: 10, h: 10, flip: true })),
    })!
    expect(s.elements.map((e) => [e.type, (e as ShapeEl).flip])).toEqual([
      ['triangle', true],
      ['parallelogram', true],
      ['hexagon', false],
      ['cloud', false],
    ])
  })

  it('keep pictures only as data:image URLs', () => {
    const files = {
      ok: { src: 'data:image/png;base64,iVBORw0KGgo=', w: 1, h: 1 },
      evil: { src: 'https://evil.example/x.png', w: 1, h: 1 },
      html: { src: 'data:text/html;base64,PHNjcmlwdD4=', w: 1, h: 1 },
    }
    const s = sanitizeScene(
      {
        version: 1,
        elements: ['ok', 'evil', 'html'].map((file) => ({ id: file, type: 'image', file, x: 0, y: 0, w: 10, h: 10 })),
      },
      files,
    )!
    expect(s.elements.map((e) => e.id)).toEqual(['ok'])
    expect(Object.keys(s.files)).toEqual(['ok'])
  })

  it('refuse what is not a scene', () => {
    for (const raw of [null, [], 'x', { version: 2, elements: [] }, { version: 1 }]) expect(sanitizeScene(raw)).toBeNull()
  })
})

describe('sketchy strokes', () => {
  it('are the same for the same seed', () => {
    expect(sketchLines([[0, 0], [100, 0], [100, 50]], 7, true)).toBe(sketchLines([[0, 0], [100, 0], [100, 50]], 7, true))
    expect(sketchEllipse(0, 0, 40, 20, 3)).toBe(sketchEllipse(0, 0, 40, 20, 3))
    expect(sketchLines([[0, 0], [100, 0]], 7)).not.toBe(sketchLines([[0, 0], [100, 0]], 8))
  })

  it('draw from a repeatable sequence', () => {
    const [a, b] = [random(42), random(42)]
    for (let i = 0; i < 5; i++) expect(a()).toBe(b())
  })
})
