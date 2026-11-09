// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { approxMeasure } from './render'
import { type Element, type Scene, add, emptyScene } from './scene'
import { drawingTitle, isDrawingPath, parseDrawing, toSVG } from './svg'

const PNG = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=='

const common = { width: 2, dash: 'solid', seed: 11 } as const
const type = { font: 'sans', size: 20, align: 'center' } as const

/** A drawing with every kind of element (the shared test file). */
function sample(sketchy: boolean): Scene {
  const els: Element[] = [
    { ...common, ...type, id: 'beds', type: 'rect', x: 0, y: 0, w: 160, h: 80, stroke: 'green', fill: 'green', rounded: true, text: 'Beds & <paths>' },
    { ...common, ...type, id: 'pond', type: 'ellipse', x: 320, y: 0, w: 140, h: 90, stroke: 'blue', fill: 'none', rounded: false, text: 'Pond', dash: 'dashed' },
    { ...common, ...type, id: 'gate', type: 'diamond', x: 160, y: 200, w: 120, h: 80, stroke: 'ink', fill: 'yellow', rounded: false, text: '' },
    {
      ...common, ...type, id: 'walk', type: 'arrow', points: [[0, 0], [1, 1]], elbow: false, start: { id: 'beds' }, end: { id: 'pond' },
      startHead: 'dot', endHead: 'triangle', stroke: 'ink', text: 'path',
    },
    {
      ...common, ...type, id: 'hose', type: 'line', points: [[0, 0], [1, 1]], elbow: true, start: { id: 'gate' }, end: { id: 'pond' },
      startHead: 'none', endHead: 'arrow', stroke: 'red', dash: 'dotted', text: '',
    },
    { ...common, ...type, id: 'note', type: 'text', x: 0, y: 320, w: 120, h: 50, stroke: 'violet', text: 'Plant in\nMay', align: 'left', font: 'serif' },
    { ...common, id: 'tree', type: 'image', x: 340, y: 220, w: 48, h: 48, stroke: 'ink', file: 'f1' },
    { ...common, id: 'twig', type: 'pen', points: [[20, 420], [30, 425], [45, 418], [60, 430]], stroke: 'orange' },
  ]
  const s = add({ ...emptyScene(), sketchy, files: { f1: { src: PNG, w: 1, h: 1 } } }, ...els)
  return s
}

describe('drawing files', () => {
  it('read back what they were written from', () => {
    for (const sketchy of [false, true]) {
      const scene = sample(sketchy)
      expect(parseDrawing(toSVG(scene, approxMeasure))).toEqual(scene)
    }
  })

  it('match the shared test files the server checks', async () => {
    // The Go tests run internal/drawing.Check on these files; after changing
    // the format, write them anew with `npx vitest run -u`.
    await expect(toSVG(sample(false), approxMeasure)).toMatchFileSnapshot('../testdata/drawing.svg')
    await expect(toSVG(sample(true), approxMeasure)).toMatchFileSnapshot('../testdata/drawing-sketchy.svg')
  })

  it('escape text and keep pictures once', () => {
    const svg = toSVG(sample(false), approxMeasure)
    expect(svg).not.toContain('<paths>')
    expect(svg).toContain('Beds &amp; &lt;paths&gt;')
    expect(svg.split(PNG).length - 1).toBe(1)
    expect(svg).toContain('<use href="#file-f1"')
  })

  it('size the canvas to the drawing', () => {
    expect(toSVG(emptyScene(), approxMeasure)).toContain('viewBox="0 0 400 300" width="400" height="300"')
    const svg = toSVG(sample(false), approxMeasure)
    const [, x, y] = /viewBox="(-?\d+) (-?\d+)/.exec(svg)!
    expect(Number(x)).toBeLessThan(0)
    expect(Number(y)).toBeLessThan(0)
  })

  it('without a scene are not drawings', () => {
    expect(parseDrawing('<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>')).toBeNull()
    expect(parseDrawing('<svg xmlns="http://www.w3.org/2000/svg"><metadata>{broken</metadata></svg>')).toBeNull()
    expect(parseDrawing('not xml')).toBeNull()
  })

  it('from elsewhere bring nothing but their scene', () => {
    const svg = `<svg xmlns="http://www.w3.org/2000/svg"><metadata id="dendronotes-drawing">{"version":1,"elements":[
      {"id":"a","type":"image","file":"x","x":0,"y":0,"w":5,"h":5},
      {"id":"b","type":"text","x":0,"y":0,"w":5,"h":5,"text":"hi","stroke":"red"}]}</metadata>
      <symbol id="file-x" viewBox="0 0 5 5"><image href="https://evil.example/t.png"/></symbol>
      <script>alert(1)</script></svg>`
    const scene = parseDrawing(svg)!
    expect(scene.elements.map((e) => e.id)).toEqual(['b'])
    expect(scene.files).toEqual({})
  })
})

describe('drawing paths', () => {
  it('end in .drawing.svg', () => {
    expect(isDrawingPath('Garden/Plan.drawing.svg')).toBe(true)
    expect(isDrawingPath('Plan.DRAWING.SVG')).toBe(true)
    expect(isDrawingPath('Plan.svg')).toBe(false)
    expect(drawingTitle('Garden/Plan.drawing.svg')).toBe('Plan')
  })
})
