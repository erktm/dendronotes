// New elements in the current style.

import type { Box, Pt } from './geometry'
import { round } from './geometry'
import { type Measure, textBox } from './render'
import {
  type Binding,
  type IconEl,
  type ImageEl,
  type LinearEl,
  type LinearType,
  type PenEl,
  type ShapeEl,
  type ShapeType,
  type TextEl,
  newId,
  newSeed,
} from './scene'
import type { Style } from './style'

const r = (p: Pt): Pt => [round(p[0]), round(p[1])]

export function newShape(type: ShapeType, box: Box, s: Style): ShapeEl {
  return {
    id: newId(), type, x: round(box.x), y: round(box.y), w: round(box.w), h: round(box.h),
    stroke: s.stroke, fill: s.fill, width: s.width, dash: s.dash, seed: newSeed(), rounded: type === 'rect' && s.rounded,
    text: '', font: s.font, size: s.size, align: s.align,
  }
}

export function newLine(type: LinearType, a: Pt, b: Pt, s: Style, start: Binding | null, end: Binding | null): LinearEl {
  return {
    id: newId(), type, points: [r(a), r(b)], elbow: s.elbow, start, end,
    startHead: type === 'arrow' ? s.startHead : 'none', endHead: type === 'arrow' ? s.endHead : 'none',
    stroke: s.stroke, width: s.width, dash: s.dash, seed: newSeed(), text: '', font: s.font, size: s.size, align: 'center',
  }
}

export function newPen(points: Pt[], s: Style): PenEl {
  return { id: newId(), type: 'pen', points: points.map(r), stroke: s.stroke, width: s.width, dash: 'solid', seed: newSeed() }
}

/** Empty text whose first line is centered on p vertically. */
export function newText(p: Pt, s: Style, measure: Measure): TextEl {
  const style = { text: '', font: s.font, size: s.size, align: s.align }
  const { w, h } = textBox(style, measure)
  return { id: newId(), type: 'text', x: round(p[0]), y: round(p[1] - h / 2), w, h, stroke: s.stroke, width: 1, dash: 'solid', seed: newSeed(), ...style }
}

export function newIcon(icon: string, box: Box, s: Style): IconEl {
  return {
    id: newId(), type: 'icon', icon, x: round(box.x), y: round(box.y), w: round(box.w), h: round(box.h),
    stroke: s.stroke, width: s.width, dash: 'solid', seed: newSeed(),
  }
}

export function newImage(file: string, box: Box): ImageEl {
  return {
    id: newId(), type: 'image', file, x: round(box.x), y: round(box.y), w: round(box.w), h: round(box.h),
    stroke: 'ink', width: 1, dash: 'solid', seed: newSeed(),
  }
}
