// The look of new elements and changing the look of selected ones: each
// element takes only what applies to it.

import type { Color, Fill } from './colors'
import { type Measure, textBox } from './render'
import { type Align, type Dash, type Element, type FontFamily, FLIPPABLE, type Head, hasText, isLinear, isShape } from './scene'

export interface Style {
  stroke: Color
  fill: Fill
  width: number
  dash: Dash
  font: FontFamily
  size: number
  align: Align
  startHead: Head
  endHead: Head
  elbow: boolean
  rounded: boolean
  flip: boolean
}

export const DEFAULT_STYLE: Style = {
  stroke: 'ink',
  fill: 'none',
  width: 2,
  dash: 'solid',
  font: 'sans',
  size: 20,
  align: 'center',
  startHead: 'none',
  endHead: 'arrow',
  elbow: false,
  rounded: true,
  flip: false,
}

export type StyleKey = keyof Style

/** The style keys that mean something for an element. */
export function keysOf(el: Element): StyleKey[] {
  const keys: StyleKey[] = []
  if (el.type !== 'image') keys.push('stroke')
  if (isShape(el)) keys.push('fill')
  if (el.type === 'rect') keys.push('rounded')
  if (isShape(el) && FLIPPABLE.has(el.type)) keys.push('flip')
  if (isShape(el) || isLinear(el) || el.type === 'pen' || el.type === 'icon') keys.push('width')
  if (isShape(el) || isLinear(el)) keys.push('dash')
  if (hasText(el)) keys.push('font', 'size')
  if (isShape(el) || el.type === 'text') keys.push('align')
  if (isLinear(el)) keys.push('startHead', 'endHead', 'elbow')
  return keys
}

/** An element with the parts of patch that apply to it (text is measured again). */
export function restyle(el: Element, patch: Partial<Style>, measure: Measure): Element {
  const keys = new Set(keysOf(el))
  const own: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(patch)) if (keys.has(k as StyleKey)) own[k] = v
  if (Object.keys(own).length === 0) return el
  const next = { ...el, ...own } as Element
  if (next.type === 'text') return { ...next, ...textBox(next, measure) }
  return next
}

/** The style an element shows, for the properties panel. */
export function styleOf(el: Element): Partial<Style> {
  const out: Partial<Style> = {}
  const rec = el as unknown as Record<string, unknown>
  for (const k of keysOf(el)) if (k in rec) (out as Record<string, unknown>)[k] = rec[k]
  return out
}
