// The scene of a drawing: what the editor changes and what a file keeps in
// its <metadata>. Elements are immutable – every operation returns a new
// scene – so the editor can keep earlier scenes for undo, and rendering can
// reuse what did not change.
//
// A scene read from a file is untrusted: sanitizeScene keeps only known
// fields with values in range, colors from the slots and pictures as
// data:image URLs.

import { type Color, type Fill, isColor } from './colors'
import { builtinIcon } from './icons'
import {
  type Box,
  type Outline,
  type Pt,
  boxOf,
  center,
  facingSide,
  outlinePoint,
  round,
  sidePoint,
  union,
} from './geometry'

export const SHAPE_TYPES = ['rect', 'ellipse', 'diamond', 'triangle', 'hexagon', 'parallelogram', 'cylinder', 'star', 'cloud'] as const
export type ShapeType = (typeof SHAPE_TYPES)[number]
/** Shapes that can be turned over: a triangle's tip down, a parallelogram leaning left. */
export const FLIPPABLE: ReadonlySet<ShapeType> = new Set(['triangle', 'parallelogram'])
export type LinearType = 'line' | 'arrow'
export type ElementType = ShapeType | LinearType | 'pen' | 'text' | 'image' | 'icon'
export type FontFamily = 'sans' | 'serif' | 'mono'
export type Align = 'left' | 'center' | 'right'
export type Dash = 'solid' | 'dashed' | 'dotted'
export type Head = 'none' | 'arrow' | 'triangle' | 'dot'

export const STROKE_WIDTHS = [1, 2, 4] as const
export const FONT_SIZES = [14, 20, 28, 40] as const
export const FONTS: readonly FontFamily[] = ['sans', 'serif', 'mono']
export const HEADS: readonly Head[] = ['none', 'arrow', 'triangle', 'dot']

interface Common {
  id: string
  /** Lines, outlines and text. */
  stroke: Color
  width: number
  dash: Dash
  /** Makes the sketchy strokes of this element repeatable. */
  seed: number
}

export interface TextStyle {
  text: string
  font: FontFamily
  size: number
  align: Align
}

export interface ShapeEl extends Common, Box, TextStyle {
  type: ShapeType
  fill: Fill
  rounded: boolean
  /** Triangles and parallelograms only (FLIPPABLE). */
  flip: boolean
}

/** Free text; w and h are the measured size of its lines. */
export interface TextEl extends Common, Box, TextStyle {
  type: 'text'
}

export interface ImageEl extends Common, Box {
  type: 'image'
  /** Key into Scene.files. */
  file: string
}

/** A built-in icon (lib/drawing/icons.ts), drawn in the stroke color. */
export interface IconEl extends Common, Box {
  type: 'icon'
  /** Name of the built-in icon. */
  icon: string
}

/** An end of a line that sticks to an element. */
export interface Binding {
  id: string
}

export interface LinearEl extends Common, TextStyle {
  type: LinearType
  /** Start and end; elbow lines are routed between them. */
  points: [Pt, Pt]
  elbow: boolean
  start: Binding | null
  end: Binding | null
  startHead: Head
  endHead: Head
}

export interface PenEl extends Common {
  type: 'pen'
  points: Pt[]
}

export type BoxEl = ShapeEl | TextEl | ImageEl | IconEl
export type Element = BoxEl | LinearEl | PenEl

/** A picture used by image elements (a data:image URL and its size). */
export interface FileData {
  src: string
  w: number
  h: number
}

export interface Scene {
  version: 1
  elements: Element[]
  files: Record<string, FileData>
  sketchy: boolean
}

export function emptyScene(): Scene {
  return { version: 1, elements: [], files: {}, sketchy: false }
}

// ---------------------------------------------------------------------------
// Kinds

const shapeTypes: ReadonlySet<string> = new Set(SHAPE_TYPES)
export const isShapeType = (t: string): t is ShapeType => shapeTypes.has(t)
export const isShape = (e: Element): e is ShapeEl => isShapeType(e.type)
export const isLinear = (e: Element): e is LinearEl => e.type === 'line' || e.type === 'arrow'
export const isBox = (e: Element): e is BoxEl => isShape(e) || e.type === 'text' || e.type === 'image' || e.type === 'icon'
/** Elements with a label or text. */
export const hasText = (e: Element): e is ShapeEl | TextEl | LinearEl => isShape(e) || e.type === 'text' || isLinear(e)

/** The outline lines stick to. */
export function outlineOf(e: BoxEl): Outline {
  if (!isShape(e)) return 'rect'
  if (e.type === 'triangle') return e.flip ? 'triangle-down' : 'triangle'
  if (e.type === 'parallelogram') return e.flip ? 'parallelogram-flip' : 'parallelogram'
  return e.type
}

export function newId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(8))
  return Array.from(bytes, (b) => (b % 36).toString(36)).join('')
}

export function newSeed(): number {
  return crypto.getRandomValues(new Uint32Array(1))[0]! % 2 ** 31
}

// ---------------------------------------------------------------------------
// Lines between elements

/** How far a bound end stays from the outline. */
export const BIND_GAP = 6

/** The points of a line as drawn: elbow lines get their corners. */
export function route(el: LinearEl, scene: Scene): Pt[] {
  const [a, b] = el.points
  if (!el.elbow) return [a, b]
  // An end leaves its element the way it sits next to it: beside it
  // horizontally, above or below it vertically.
  const leavesHorizontally = (bind: Binding | null, p: Pt, other: Pt): boolean => {
    const target = bind && find(scene, bind.id)
    if (target && isBox(target)) {
      if (p[0] <= target.x || p[0] >= target.x + target.w) return true
      if (p[1] <= target.y || p[1] >= target.y + target.h) return false
    }
    return Math.abs(other[0] - p[0]) >= Math.abs(other[1] - p[1])
  }
  const h0 = leavesHorizontally(el.start, a, b)
  const h1 = leavesHorizontally(el.end, b, a)
  if (h0 && h1) {
    const mx = (a[0] + b[0]) / 2
    return dedupe([a, [mx, a[1]], [mx, b[1]], b])
  }
  if (!h0 && !h1) {
    const my = (a[1] + b[1]) / 2
    return dedupe([a, [a[0], my], [b[0], my], b])
  }
  return dedupe(h0 ? [a, [b[0], a[1]], b] : [a, [a[0], b[1]], b])
}

function dedupe(points: Pt[]): Pt[] {
  return points.filter((p, i) => i === 0 || p[0] !== points[i - 1]![0] || p[1] !== points[i - 1]![1])
}

/** Where an end bound to target goes, given the point the line comes from. */
export function endPoint(target: BoxEl, from: Pt, elbow: boolean): Pt {
  const [x, y] = elbow ? sidePoint(target, facingSide(target, from), BIND_GAP) : outlinePoint(target, outlineOf(target), from, BIND_GAP)
  return [round(x), round(y)]
}

/** Moves the bound ends of a line to their elements. */
export function attach(el: LinearEl, scene: Scene): LinearEl {
  const startEl = el.start && find(scene, el.start.id)
  const endEl = el.end && find(scene, el.end.id)
  const s = startEl && isBox(startEl) ? startEl : null
  const t = endEl && isBox(endEl) ? endEl : null
  if (!s && !t) return el.start || el.end ? { ...el, start: null, end: null } : el
  let [a, b] = el.points
  // Each end aims at the other element's center, or at the other free end.
  const aimA: Pt = t ? center(t) : b
  const aimB: Pt = s ? center(s) : a
  if (s) a = endPoint(s, aimA, el.elbow)
  if (t) b = endPoint(t, aimB, el.elbow)
  const same = (p: Pt, q: Pt) => p[0] === q[0] && p[1] === q[1]
  const start = s ? el.start : null
  const end = t ? el.end : null
  if (same(a, el.points[0]) && same(b, el.points[1]) && start === el.start && end === el.end) return el
  return { ...el, points: [a, b], start, end }
}

/** Reattaches every line bound to one of ids (all lines without ids). */
export function reattach(scene: Scene, ids?: ReadonlySet<string>): Scene {
  let changed = false
  const elements = scene.elements.map((e) => {
    if (!isLinear(e) || !(e.start || e.end)) return e
    if (ids && !(e.start && ids.has(e.start.id)) && !(e.end && ids.has(e.end.id)) && !ids.has(e.id)) return e
    const next = attach(e, scene)
    if (next !== e) changed = true
    return next
  })
  return changed ? { ...scene, elements } : scene
}

// ---------------------------------------------------------------------------
// Queries

export function find(scene: Scene, id: string): Element | undefined {
  return scene.elements.find((e) => e.id === id)
}

/** The bounding box of an element as drawn (without the stroke width). */
export function bounds(el: Element, scene: Scene): Box {
  if (isBox(el)) return { x: el.x, y: el.y, w: el.w, h: el.h }
  if (isLinear(el)) return boxOf(route(el, scene))
  return boxOf(el.points)
}

export function sceneBounds(scene: Scene, ids?: ReadonlySet<string>): Box | null {
  return union(scene.elements.filter((e) => !ids || ids.has(e.id)).map((e) => bounds(e, scene)))
}

// ---------------------------------------------------------------------------
// Operations

export function add(scene: Scene, ...els: Element[]): Scene {
  return reattach({ ...scene, elements: [...scene.elements, ...els] }, new Set(els.map((e) => e.id)))
}

/** Replaces elements by id and moves the lines bound to them along. */
export function replace(scene: Scene, ...els: Element[]): Scene {
  const byId = new Map(els.map((e) => [e.id, e]))
  const elements = scene.elements.map((e) => byId.get(e.id) ?? e)
  return reattach({ ...scene, elements }, new Set(byId.keys()))
}

export function update<E extends Element>(scene: Scene, el: E, patch: Partial<E>): Scene {
  return replace(scene, { ...el, ...patch })
}

function translate(el: Element, dx: number, dy: number): Element {
  if (isBox(el)) return { ...el, x: round(el.x + dx), y: round(el.y + dy) }
  if (isLinear(el)) {
    const [a, b] = el.points
    return { ...el, points: [[round(a[0] + dx), round(a[1] + dy)], [round(b[0] + dx), round(b[1] + dy)]] }
  }
  return { ...el, points: el.points.map(([x, y]): Pt => [round(x + dx), round(y + dy)]) }
}

/**
 * Moves elements. A line keeps an end bound only if the element it sticks
 * to moves along; lines bound to moved elements follow them.
 */
export function move(scene: Scene, ids: ReadonlySet<string>, dx: number, dy: number): Scene {
  const elements = scene.elements.map((e) => {
    if (!ids.has(e.id)) return e
    let moved = translate(e, dx, dy)
    if (isLinear(moved)) {
      const keep = (b: Binding | null) => (b && ids.has(b.id) ? b : null)
      moved = { ...moved, start: keep(moved.start), end: keep(moved.end) }
    }
    return moved
  })
  return reattach({ ...scene, elements }, ids)
}

/** Removes elements; lines bound to them keep their place but come loose. */
export function remove(scene: Scene, ids: ReadonlySet<string>): Scene {
  const elements = scene.elements
    .filter((e) => !ids.has(e.id))
    .map((e) => {
      if (!isLinear(e)) return e
      const start = e.start && ids.has(e.start.id) ? null : e.start
      const end = e.end && ids.has(e.end.id) ? null : e.end
      return start === e.start && end === e.end ? e : { ...e, start, end }
    })
  return { ...scene, elements }
}

/** Copies of elements with new ids, moved by offset; bindings among them are kept. */
export function copies(els: readonly Element[], offset: number): Element[] {
  const ids = new Map(els.map((e) => [e.id, newId()]))
  return els.map((e) => {
    let c = translate({ ...e, id: ids.get(e.id)!, seed: newSeed() }, offset, offset)
    if (isLinear(c)) {
      const remap = (b: Binding | null) => (b && ids.has(b.id) ? { id: ids.get(b.id)! } : null)
      c = { ...c, start: remap(c.start), end: remap(c.end) }
    }
    return c
  })
}

export type Order = 'front' | 'back' | 'forward' | 'backward'

/** Changes the stacking order of elements (later is in front). */
export function reorder(scene: Scene, ids: ReadonlySet<string>, order: Order): Scene {
  const els = [...scene.elements]
  const picked = els.filter((e) => ids.has(e.id))
  const rest = els.filter((e) => !ids.has(e.id))
  if (order === 'front') return { ...scene, elements: [...rest, ...picked] }
  if (order === 'back') return { ...scene, elements: [...picked, ...rest] }
  if (order === 'forward') {
    for (let i = els.length - 2; i >= 0; i--) {
      if (ids.has(els[i]!.id) && !ids.has(els[i + 1]!.id)) [els[i], els[i + 1]] = [els[i + 1]!, els[i]!]
    }
  } else {
    for (let i = 1; i < els.length; i++) {
      if (ids.has(els[i]!.id) && !ids.has(els[i - 1]!.id)) [els[i], els[i - 1]] = [els[i - 1]!, els[i]!]
    }
  }
  return { ...scene, elements: els }
}

/** Fits an element into a new box (shapes, text and pictures take it; lines scale). */
export function fit(el: Element, from: Box, to: Box): Element {
  const sx = from.w > 0 ? to.w / from.w : 1
  const sy = from.h > 0 ? to.h / from.h : 1
  const map = ([x, y]: Pt): Pt => [round(to.x + (x - from.x) * sx), round(to.y + (y - from.y) * sy)]
  if (isBox(el)) {
    const [x, y] = map([el.x, el.y])
    return { ...el, x, y, w: round(Math.max(1, el.w * sx)), h: round(Math.max(1, el.h * sy)) }
  }
  if (isLinear(el)) return { ...el, points: [map(el.points[0]), map(el.points[1])] }
  return { ...el, points: el.points.map(map) }
}

/** Drops pictures no element uses any more. */
export function pruneFiles(scene: Scene): Scene {
  const used = new Set(scene.elements.flatMap((e) => (e.type === 'image' ? [e.file] : [])))
  const keys = Object.keys(scene.files)
  if (keys.every((k) => used.has(k))) return scene
  return { ...scene, files: Object.fromEntries(keys.filter((k) => used.has(k)).map((k) => [k, scene.files[k]!])) }
}

// ---------------------------------------------------------------------------
// Untrusted scenes

export const LIMITS = {
  elements: 5000,
  penPoints: 5000,
  text: 10000,
  coordinate: 1e6,
  size: 1e6,
  files: 300,
}

const ID = /^[A-Za-z0-9_-]{1,40}$/
export const DATA_IMAGE = /^data:image\/(png|jpeg|webp|gif|svg\+xml);base64,[A-Za-z0-9+/=]+$/
// Characters XML does not allow, kept out of every text.
// eslint-disable-next-line no-control-regex
const NOT_XML = /[\u0000-\u0008\u000B\u000C\u000E-\u001F￾￿]/g

const obj = (v: unknown): Record<string, unknown> | null =>
  typeof v === 'object' && v !== null && !Array.isArray(v) ? (v as Record<string, unknown>) : null
const num = (v: unknown, lo: number, hi: number, dflt: number) =>
  typeof v === 'number' && Number.isFinite(v) ? Math.min(hi, Math.max(lo, v)) : dflt
const coord = (v: unknown) => num(v, -LIMITS.coordinate, LIMITS.coordinate, 0)
const oneOf = <T extends string>(v: unknown, options: readonly T[], dflt: T): T =>
  typeof v === 'string' && (options as readonly string[]).includes(v) ? (v as T) : dflt
const text = (v: unknown) => (typeof v === 'string' ? v.slice(0, LIMITS.text).replace(NOT_XML, '') : '')

function point(v: unknown): Pt | null {
  if (!Array.isArray(v) || v.length !== 2) return null
  return [coord(v[0]), coord(v[1])]
}

function common(o: Record<string, unknown>, id: string) {
  return {
    id,
    stroke: isColor(o.stroke) ? o.stroke : ('ink' as Color),
    width: num(o.width, 0.5, 16, 2),
    dash: oneOf(o.dash, ['solid', 'dashed', 'dotted'] as const, 'solid'),
    seed: Math.floor(num(o.seed, 0, 2 ** 31 - 1, 1)),
  }
}

function textStyle(o: Record<string, unknown>): TextStyle {
  return {
    text: text(o.text),
    font: oneOf(o.font, FONTS, 'sans'),
    size: num(o.size, 6, 200, 20),
    align: oneOf(o.align, ['left', 'center', 'right'] as const, 'center'),
  }
}

function box(o: Record<string, unknown>): Box {
  return { x: coord(o.x), y: coord(o.y), w: num(o.w, 0, LIMITS.size, 0), h: num(o.h, 0, LIMITS.size, 0) }
}

function sanitizeElement(v: unknown, files: Record<string, FileData>): Element | null {
  const o = obj(v)
  if (!o || typeof o.id !== 'string' || !ID.test(o.id)) return null
  const base = common(o, o.id)
  switch (o.type) {
    case 'rect':
    case 'ellipse':
    case 'diamond':
    case 'triangle':
    case 'hexagon':
    case 'parallelogram':
    case 'cylinder':
    case 'star':
    case 'cloud':
      return {
        ...base, ...box(o), ...textStyle(o), type: o.type,
        fill: o.fill === 'none' || !isColor(o.fill) ? 'none' : o.fill,
        rounded: o.rounded === true,
        flip: o.flip === true && FLIPPABLE.has(o.type),
      }
    case 'text':
      return { ...base, ...box(o), ...textStyle(o), type: 'text', align: oneOf(o.align, ['left', 'center', 'right'] as const, 'left') }
    case 'image':
      if (typeof o.file !== 'string' || !files[o.file]) return null
      return { ...base, ...box(o), type: 'image', file: o.file }
    case 'icon':
      // Only icons dendronotes knows: the picture comes from it, never from the file.
      if (typeof o.icon !== 'string' || !builtinIcon(o.icon)) return null
      return { ...base, ...box(o), type: 'icon', icon: o.icon }
    case 'line':
    case 'arrow': {
      const pts = Array.isArray(o.points) ? o.points.map(point) : []
      if (pts.length < 2 || !pts[0] || !pts[pts.length - 1]) return null
      const bind = (b: unknown): Binding | null => {
        const bo = obj(b)
        return bo && typeof bo.id === 'string' && ID.test(bo.id) ? { id: bo.id } : null
      }
      return {
        ...base, ...textStyle(o), type: o.type,
        points: [pts[0], pts[pts.length - 1]!],
        elbow: o.elbow === true,
        start: bind(o.start),
        end: bind(o.end),
        startHead: oneOf(o.startHead, HEADS, 'none'),
        endHead: oneOf(o.endHead, HEADS, o.type === 'arrow' ? 'arrow' : 'none'),
      }
    }
    case 'pen': {
      const pts = Array.isArray(o.points) ? o.points.slice(0, LIMITS.penPoints).map(point).filter((p): p is Pt => p !== null) : []
      if (pts.length === 0) return null
      return { ...base, type: 'pen', points: pts }
    }
  }
  return null
}

/** A safe scene from parsed JSON (and the pictures found in the file), or null. */
export function sanitizeScene(raw: unknown, files: Record<string, FileData> = {}): Scene | null {
  const o = obj(raw)
  if (!o || o.version !== 1 || !Array.isArray(o.elements)) return null
  const safeFiles: Record<string, FileData> = {}
  for (const [k, f] of Object.entries(files).slice(0, LIMITS.files)) {
    if (ID.test(k) && DATA_IMAGE.test(f.src)) safeFiles[k] = { src: f.src, w: num(f.w, 1, 1e4, 1), h: num(f.h, 1, 1e4, 1) }
  }
  const seen = new Set<string>()
  const elements: Element[] = []
  for (const v of o.elements.slice(0, LIMITS.elements)) {
    const el = sanitizeElement(v, safeFiles)
    if (!el || seen.has(el.id)) continue
    seen.add(el.id)
    elements.push(el)
  }
  // Bindings only to elements lines can stick to.
  const boxes = new Set(elements.filter(isBox).map((e) => e.id))
  const clean = elements.map((e) => {
    if (!isLinear(e)) return e
    const ok = (b: Binding | null) => (b && boxes.has(b.id) ? b : null)
    return { ...e, start: ok(e.start), end: ok(e.end) }
  })
  return pruneFiles({ version: 1, elements: clean, files: safeFiles, sketchy: o.sketchy === true })
}
