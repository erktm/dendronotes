// What the pointer is on in the editor: elements, the elements lines may
// stick to, and the handles of a selection.

import { type Box, type Pt, contains, grow, inBox, insideOutline, outlineDist, polylineDist } from './geometry'
import { type Measure, lineLabelBox } from './render'
import { type BoxEl, type Element, type Scene, bounds, isBox, isLinear, isShape, outlineOf, route } from './scene'

/** The topmost element at p; tol is the reach in scene units. */
export function hitTest(scene: Scene, p: Pt, tol: number, measure: Measure): Element | null {
  for (let i = scene.elements.length - 1; i >= 0; i--) {
    const el = scene.elements[i]!
    if (hits(el, scene, p, tol + el.width / 2, measure)) return el
  }
  return null
}

function hits(el: Element, scene: Scene, p: Pt, tol: number, measure: Measure): boolean {
  if (isShape(el)) {
    // Empty shapes are picked at their outline, so what lies inside stays reachable.
    if (el.fill !== 'none' || el.text.trim() !== '') return insideOutline(p, grow(el, tol), outlineOf(el))
    return outlineDist(p, el, outlineOf(el)) <= tol
  }
  if (isBox(el)) return inBox(p, grow(el, tol))
  if (isLinear(el)) {
    const label = lineLabelBox(el, scene, measure)
    return polylineDist(p, route(el, scene)) <= tol || (label !== null && inBox(p, label))
  }
  return polylineDist(p, el.points) <= tol
}

/** The topmost shape p lies inside of (also empty ones). */
export function shapeAt(scene: Scene, p: Pt): Element | null {
  for (let i = scene.elements.length - 1; i >= 0; i--) {
    const el = scene.elements[i]!
    if (isShape(el) && insideOutline(p, el, outlineOf(el))) return el
  }
  return null
}

/** The element a line end at p sticks to, if any. */
export function bindTargetAt(scene: Scene, p: Pt, tol: number, exclude: ReadonlySet<string>): BoxEl | null {
  for (let i = scene.elements.length - 1; i >= 0; i--) {
    const el = scene.elements[i]!
    if (!isBox(el) || exclude.has(el.id)) continue
    if (insideOutline(p, grow(el, tol), outlineOf(el))) return el
  }
  return null
}

/** The ids of the elements completely inside box. */
export function elementsIn(scene: Scene, box: Box): string[] {
  return scene.elements.filter((e) => contains(box, bounds(e, scene))).map((e) => e.id)
}

export type Handle = 'nw' | 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w'
export const CORNERS: readonly Handle[] = ['nw', 'ne', 'se', 'sw']
export const ALL_HANDLES: readonly Handle[] = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w']

/** Where a handle sits on a box. */
export function handlePoint(b: Box, h: Handle): Pt {
  const x = h.includes('w') ? b.x : h.includes('e') ? b.x + b.w : b.x + b.w / 2
  const y = h.includes('n') ? b.y : h.includes('s') ? b.y + b.h : b.y + b.h / 2
  return [x, y]
}

/**
 * The box after dragging one of its handles to p; the opposite side stays.
 * With keepRatio (Shift, or a corner of several elements) the proportions stay.
 */
export function resizeBox(b: Box, h: Handle, p: Pt, keepRatio: boolean, min = 4): Box {
  let x0 = b.x
  let y0 = b.y
  let x1 = b.x + b.w
  let y1 = b.y + b.h
  if (h.includes('w')) x0 = Math.min(p[0], x1 - min)
  if (h.includes('e')) x1 = Math.max(p[0], x0 + min)
  if (h.includes('n')) y0 = Math.min(p[1], y1 - min)
  if (h.includes('s')) y1 = Math.max(p[1], y0 + min)
  if (keepRatio && b.w > 0 && b.h > 0) {
    const ratio = b.w / b.h
    const corner = h.length === 2
    let w = x1 - x0
    let hh = y1 - y0
    if (corner) {
      if (w / hh > ratio) hh = w / ratio
      else w = hh * ratio
    } else if (h === 'e' || h === 'w') {
      hh = w / ratio
    } else {
      w = hh * ratio
    }
    if (h.includes('w')) x0 = x1 - w
    else if (h.includes('e') || !corner) x1 = h === 'n' || h === 's' ? b.x + b.w / 2 + w / 2 : x0 + w
    if (h === 'n' || h === 's') x0 = b.x + b.w / 2 - w / 2
    if (h.includes('n')) y0 = y1 - hh
    else if (h.includes('s') || !corner) y1 = h === 'e' || h === 'w' ? b.y + b.h / 2 + hh / 2 : y0 + hh
    if (h === 'e' || h === 'w') y0 = b.y + b.h / 2 - hh / 2
  }
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 }
}
