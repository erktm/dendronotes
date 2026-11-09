// Drawings in the app: the primitives of each element as SVG DOM, built with
// createElementNS and textContent only – never from markup – so a drawing
// from a file cannot bring anything along. Colors come from the classes in
// styles/drawing.css (tokens), not from the file's own style sheet.

import { type Measure, type Prim, FONT_FAMILIES, approxMeasure, primitives } from './render'
import type { Element, Scene } from './scene'

export const SVG_NS = 'http://www.w3.org/2000/svg'

function primNode(p: Prim, scene: Scene): SVGElement | null {
  if (p.tag === 'image') {
    const file = p.file ? scene.files[p.file] : undefined
    if (!file) return null
    const img = document.createElementNS(SVG_NS, 'image')
    for (const [k, v] of Object.entries(p.attrs)) img.setAttribute(k, v)
    img.setAttribute('href', file.src) // a data:image URL (sanitizeScene)
    return img
  }
  const node = document.createElementNS(SVG_NS, p.tag)
  for (const [k, v] of Object.entries(p.attrs)) node.setAttribute(k, v)
  for (const l of p.lines ?? []) {
    const span = document.createElementNS(SVG_NS, 'tspan')
    span.setAttribute('x', String(l.x))
    span.setAttribute('y', String(l.y))
    span.textContent = l.text
    node.appendChild(span)
  }
  return node
}

/** One element as a <g data-id>. */
export function elementNode(el: Element, scene: Scene, measure: Measure): SVGGElement {
  const g = document.createElementNS(SVG_NS, 'g')
  g.dataset.id = el.id
  for (const p of primitives(el, scene, measure)) {
    const n = primNode(p, scene)
    if (n) g.appendChild(n)
  }
  return g
}

/**
 * Keeps the children of a <g> in step with a scene, rebuilding only the
 * elements that changed (elements are immutable, so a changed one is a new
 * object).
 */
export class SceneLayer {
  private nodes = new Map<string, { el: Element; node: SVGGElement }>()
  private sketchy = false

  constructor(
    private root: SVGGElement,
    private measure: Measure,
  ) {}

  render(scene: Scene) {
    if (scene.sketchy !== this.sketchy) {
      this.nodes.clear()
      this.sketchy = scene.sketchy
    }
    const next = new Map<string, { el: Element; node: SVGGElement }>()
    const ordered: SVGGElement[] = []
    for (const el of scene.elements) {
      const known = this.nodes.get(el.id)
      const node = known && known.el === el ? known.node : elementNode(el, scene, this.measure)
      next.set(el.id, { el, node })
      ordered.push(node)
    }
    this.nodes = next
    this.root.replaceChildren(...ordered)
  }

  /** After the fonts arrive, text has to be measured again. */
  refresh(scene: Scene) {
    this.nodes.clear()
    this.render(scene)
  }
}

/** Measures text with a canvas (falls back to a guess without one). */
export function canvasMeasure(): Measure {
  const ctx = typeof document !== 'undefined' ? document.createElement('canvas').getContext('2d') : null
  if (!ctx) return approxMeasure
  const cache = new Map<string, number>()
  return (text, font, size) => {
    const key = `${font}:${size}:${text}`
    let w = cache.get(key)
    if (w === undefined) {
      ctx.font = `${size}px ${FONT_FAMILIES[font]}`
      w = ctx.measureText(text).width
      if (cache.size > 5000) cache.clear()
      cache.set(key, w)
    }
    return w
  }
}

let shared: Measure | null = null
/** One canvas measure for the whole app. */
export function measureText(): Measure {
  shared ??= canvasMeasure()
  return shared
}
