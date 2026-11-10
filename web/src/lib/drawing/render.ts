// What an element looks like: a short list of SVG primitives. The same
// primitives become the markup of a saved file (svg.ts) and the DOM in the
// app (dom.ts), so a drawing looks the same in both.

import { type Color, PAPER_CLASS, solidClass, strokeClass, tintClass } from './colors'
import { type Box, type Pt, center, diamondPoints, round } from './geometry'
import {
  type Dash,
  type Element,
  type FontFamily,
  type Head,
  type LinearEl,
  type Scene,
  type ShapeEl,
  type TextEl,
  type TextStyle,
  route,
} from './scene'
import { sketchEllipse, sketchLines, sketchRoundRect, smooth } from './sketch'
import { builtinIcon } from './icons'

export interface TextLine {
  x: number
  y: number
  text: string
}

export interface Prim {
  tag: 'rect' | 'ellipse' | 'path' | 'circle' | 'text' | 'image'
  attrs: Record<string, string>
  /** text: one tspan per line. */
  lines?: TextLine[]
  /** image: the picture (a key into Scene.files). */
  file?: string
}

/** Width of a text in pixels at a font and size. */
export type Measure = (text: string, font: FontFamily, size: number) => number

/** A rough guess without a browser (tests, or when no canvas is available). */
export const approxMeasure: Measure = (text, font, size) => [...text].length * size * (font === 'mono' ? 0.6 : 0.53)

export const FONT_FAMILIES: Record<FontFamily, string> = {
  sans: "'Inter Variable', Inter, ui-sans-serif, system-ui, sans-serif",
  serif: "'Source Serif 4 Variable', 'Source Serif 4', Georgia, serif",
  mono: "'JetBrains Mono Variable', 'JetBrains Mono', ui-monospace, Menlo, monospace",
}

export const LINE_HEIGHT = 1.25
/** Space between a shape's outline and its label. */
const PADDING = 8

const f = (n: number) => String(round(n))
const fine = (n: number) => String(Math.round(n * 10000) / 10000)

// ---------------------------------------------------------------------------
// Text

/** Splits text into lines, wrapping words at maxWidth when given. */
export function wrap(text: string, font: FontFamily, size: number, maxWidth: number | null, measure: Measure): string[] {
  const out: string[] = []
  for (const para of text.split('\n')) {
    if (maxWidth === null || measure(para, font, size) <= maxWidth) {
      out.push(para)
      continue
    }
    let line = ''
    for (const word of para.split(/(\s+)/)) {
      const next = line + word
      if (line !== '' && measure(next.trimEnd(), font, size) > maxWidth) {
        out.push(line.trimEnd())
        line = word.trimStart()
      } else {
        line = next
      }
      // A word longer than the line is broken between characters.
      while (measure(line, font, size) > maxWidth && [...line].length > 1) {
        const chars = [...line]
        let n = chars.length - 1
        while (n > 1 && measure(chars.slice(0, n).join(''), font, size) > maxWidth) n--
        out.push(chars.slice(0, n).join(''))
        line = chars.slice(n).join('')
      }
    }
    out.push(line.trimEnd())
  }
  return out
}

/** The size of free text. */
export function textBox(style: TextStyle, measure: Measure): { w: number; h: number } {
  const lines = style.text.split('\n')
  const w = Math.max(...lines.map((l) => measure(l, style.font, style.size)), style.size / 2)
  return { w: Math.ceil(w), h: Math.ceil(lines.length * style.size * LINE_HEIGHT) }
}

/** The lines of a text block, vertically centered on cy, aligned within [left, left+width]. */
function textPrim(style: TextStyle, lines: string[], left: number, width: number, top: number, color: Color): Prim {
  const lh = style.size * LINE_HEIGHT
  const anchor = style.align === 'left' ? 'start' : style.align === 'right' ? 'end' : 'middle'
  const x = style.align === 'left' ? left : style.align === 'right' ? left + width : left + width / 2
  return {
    tag: 'text',
    attrs: {
      class: solidClass(color),
      'font-family': FONT_FAMILIES[style.font],
      'font-size': f(style.size),
      'text-anchor': anchor,
    },
    // The baseline of each line sits a little below the middle of its line box.
    lines: lines.map((text, i) => ({ x: round(x), y: round(top + i * lh + lh / 2 + style.size * 0.35), text })),
  }
}

/** The area inside a shape that its label may fill. */
function labelArea(el: ShapeEl): Box {
  const k = el.type === 'ellipse' ? 0.72 : el.type === 'diamond' ? 0.5 : 1
  const w = Math.max(el.w * k - 2 * PADDING, el.size)
  const [cx, cy] = center(el)
  return { x: cx - w / 2, y: cy - el.h / 2, w, h: el.h }
}

export function shapeLabel(el: ShapeEl, measure: Measure): Prim | null {
  if (el.text.trim() === '') return null
  const area = labelArea(el)
  const lines = wrap(el.text, el.font, el.size, area.w, measure)
  const height = lines.length * el.size * LINE_HEIGHT
  return textPrim(el, lines, area.x, area.w, area.y + (area.h - height) / 2, el.stroke)
}

/** The middle of a line by length. */
export function midpoint(points: readonly Pt[]): Pt {
  let total = 0
  for (let i = 1; i < points.length; i++) total += Math.hypot(points[i]![0] - points[i - 1]![0], points[i]![1] - points[i - 1]![1])
  let half = total / 2
  for (let i = 1; i < points.length; i++) {
    const a = points[i - 1]!
    const b = points[i]!
    const len = Math.hypot(b[0] - a[0], b[1] - a[1])
    if (half <= len && len > 0) return [a[0] + ((b[0] - a[0]) * half) / len, a[1] + ((b[1] - a[1]) * half) / len]
    half -= len
  }
  return points[0] ?? [0, 0]
}

/** The box of a line's label (for hits and the knock-out behind it). */
export function lineLabelBox(el: LinearEl, scene: Scene, measure: Measure): Box | null {
  if (el.text.trim() === '') return null
  const lines = el.text.split('\n')
  const w = Math.max(...lines.map((l) => measure(l, el.font, el.size)))
  const h = lines.length * el.size * LINE_HEIGHT
  const [mx, my] = midpoint(route(el, scene))
  return { x: mx - w / 2 - 4, y: my - h / 2 - 2, w: w + 8, h: h + 4 }
}

// ---------------------------------------------------------------------------
// Strokes

function strokeAttrs(color: Color, width: number, dash: Dash): Record<string, string> {
  const a: Record<string, string> = { class: strokeClass(color), 'stroke-width': f(width) }
  if (dash === 'dashed') a['stroke-dasharray'] = `${f(width * 4)} ${f(width * 3)}`
  if (dash === 'dotted') {
    a['stroke-dasharray'] = `0 ${f(width * 2.5)}`
    a['stroke-linecap'] = 'round'
  }
  return a
}

function headPrims(head: Head, tip: Pt, from: Pt, color: Color, width: number): Prim[] {
  if (head === 'none') return []
  const angle = Math.atan2(tip[1] - from[1], tip[0] - from[0])
  if (head === 'dot') {
    return [{ tag: 'circle', attrs: { class: solidClass(color), cx: f(tip[0]), cy: f(tip[1]), r: f(2.5 + width) } }]
  }
  const len = 8 + width * 3
  const spread = head === 'triangle' ? 0.42 : 0.5
  const side = (s: number): Pt => [tip[0] - Math.cos(angle + s * spread) * len, tip[1] - Math.sin(angle + s * spread) * len]
  const [l, r] = [side(1), side(-1)]
  const d = `M${f(l[0])} ${f(l[1])}L${f(tip[0])} ${f(tip[1])}L${f(r[0])} ${f(r[1])}`
  const attrs = { class: strokeClass(color), 'stroke-width': f(width), 'stroke-linejoin': 'round', 'stroke-linecap': 'round' }
  if (head === 'triangle') return [{ tag: 'path', attrs: { ...attrs, class: `${strokeClass(color)} ${solidClass(color)}`, d: d + 'Z' } }]
  return [{ tag: 'path', attrs: { ...attrs, fill: 'none', d } }]
}

function polyline(points: readonly Pt[]): string {
  return 'M' + points.map((p) => `${f(p[0])} ${f(p[1])}`).join('L')
}

// ---------------------------------------------------------------------------
// Elements

/** The plain outline of a shape (tag and geometry). */
const cornerRadius = (el: ShapeEl) => Math.min(16, Math.min(el.w, el.h) * 0.2)

function outlinePrim(el: ShapeEl): Prim {
  const r = el.rounded ? cornerRadius(el) : 0
  if (el.type === 'rect') {
    const attrs: Record<string, string> = { x: f(el.x), y: f(el.y), width: f(el.w), height: f(el.h) }
    if (r) attrs.rx = f(r)
    return { tag: 'rect', attrs }
  }
  if (el.type === 'ellipse') {
    const [cx, cy] = center(el)
    return { tag: 'ellipse', attrs: { cx: f(cx), cy: f(cy), rx: f(el.w / 2), ry: f(el.h / 2) } }
  }
  return { tag: 'path', attrs: { 'stroke-linejoin': 'round', d: polyline(diamondPoints(el)) + 'Z' } }
}

function shapePrims(el: ShapeEl, sketchy: boolean, measure: Measure): Prim[] {
  const out: Prim[] = []
  const stroke = strokeAttrs(el.stroke, el.width, el.dash)
  const shape = outlinePrim(el)
  if (!sketchy) {
    const attrs: Record<string, string> = { ...stroke, ...shape.attrs }
    if (el.fill === 'none') attrs.fill = 'none'
    else attrs.class = `${stroke.class} ${tintClass(el.fill)}`
    out.push({ tag: shape.tag, attrs })
  } else {
    const [cx, cy] = center(el)
    if (el.fill !== 'none') out.push({ tag: shape.tag, attrs: { ...shape.attrs, class: tintClass(el.fill) } })
    const corners: Pt[] =
      el.type === 'diamond'
        ? diamondPoints(el)
        : [[el.x, el.y], [el.x + el.w, el.y], [el.x + el.w, el.y + el.h], [el.x, el.y + el.h]]
    const r = el.rounded && el.type === 'rect' ? cornerRadius(el) : 0
    const d =
      el.type === 'ellipse'
        ? sketchEllipse(cx, cy, el.w / 2, el.h / 2, el.seed)
        : r > 0
          ? sketchRoundRect(el.x, el.y, el.w, el.h, r, el.seed)
          : sketchLines(corners, el.seed, true)
    out.push({ tag: 'path', attrs: { ...stroke, fill: 'none', 'stroke-linecap': 'round', d } })
  }
  const label = shapeLabel(el, measure)
  if (label) out.push(label)
  return out
}

function linearPrims(el: LinearEl, scene: Scene, sketchy: boolean, measure: Measure): Prim[] {
  const pts = route(el, scene)
  const d = sketchy ? sketchLines(pts, el.seed, false, 1.5) : polyline(pts)
  const out: Prim[] = [
    { tag: 'path', attrs: { ...strokeAttrs(el.stroke, el.width, el.dash), fill: 'none', 'stroke-linejoin': 'round', 'stroke-linecap': 'round', d } },
  ]
  if (pts.length >= 2) {
    out.push(...headPrims(el.startHead, pts[0]!, pts[1]!, el.stroke, el.width))
    out.push(...headPrims(el.endHead, pts[pts.length - 1]!, pts[pts.length - 2]!, el.stroke, el.width))
  }
  const box = lineLabelBox(el, scene, measure)
  if (box) {
    out.push({ tag: 'rect', attrs: { class: PAPER_CLASS, x: f(box.x), y: f(box.y), width: f(box.w), height: f(box.h), rx: '4' } })
    out.push(textPrim({ ...el, align: 'center' }, el.text.split('\n'), box.x, box.w, box.y + 2, el.stroke))
  }
  return out
}

function textElPrims(el: TextEl): Prim[] {
  if (el.text === '') return []
  return [textPrim(el, el.text.split('\n'), el.x, el.w, el.y, el.stroke)]
}

/** The primitives of one element. */
export function primitives(el: Element, scene: Scene, measure: Measure): Prim[] {
  switch (el.type) {
    case 'rect':
    case 'ellipse':
    case 'diamond':
      return shapePrims(el, scene.sketchy, measure)
    case 'line':
    case 'arrow':
      return linearPrims(el, scene, scene.sketchy, measure)
    case 'text':
      return textElPrims(el)
    case 'image':
      return [{ tag: 'image', file: el.file, attrs: { x: f(el.x), y: f(el.y), width: f(el.w), height: f(el.h) } }]
    case 'icon': {
      const icon = builtinIcon(el.icon)
      if (!icon) return []
      // Drawn on its 24-unit grid, scaled to the box; the line keeps its width.
      const sx = el.w / 24
      const sy = el.h / 24
      const scale = Math.max(Math.min(sx, sy), 0.01)
      return [
        {
          tag: 'path',
          attrs: {
            class: strokeClass(el.stroke),
            fill: 'none',
            'stroke-width': f(el.width / scale),
            'stroke-linecap': 'round',
            'stroke-linejoin': 'round',
            transform: `translate(${f(el.x)} ${f(el.y)}) scale(${fine(sx)} ${fine(sy)})`,
            d: icon.d,
          },
        },
      ]
    }
    case 'pen': {
      const attrs = { class: strokeClass(el.stroke), 'stroke-width': f(el.width), fill: 'none', 'stroke-linecap': 'round', 'stroke-linejoin': 'round' }
      if (el.points.length === 1) {
        const [x, y] = el.points[0]!
        return [{ tag: 'circle', attrs: { class: solidClass(el.stroke), cx: f(x), cy: f(y), r: f(el.width / 2 + 0.5) } }]
      }
      return [{ tag: 'path', attrs: { ...attrs, d: smooth(el.points) } }]
    }
  }
}

/** The colors a scene uses (for the style sheet of a file). */
export function usedColors(scene: Scene): Set<Color> {
  const out = new Set<Color>()
  for (const e of scene.elements) {
    out.add(e.stroke)
    if ((e.type === 'rect' || e.type === 'ellipse' || e.type === 'diamond') && e.fill !== 'none') out.add(e.fill)
  }
  return out
}
