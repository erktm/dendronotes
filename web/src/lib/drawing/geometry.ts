// Plane geometry for drawings: boxes, distances and where a line meets the
// outline of a shape.

export type Pt = [number, number]

export interface Box {
  x: number
  y: number
  w: number
  h: number
}

/**
 * The outline a shape has within its box. Rectangles and ellipses are exact;
 * every other outline is a polygon (outlinePolygon): the corners of the
 * straight-edged shapes, points along the curves of cylinder and cloud.
 */
export type Outline =
  | 'rect'
  | 'ellipse'
  | 'diamond'
  | 'triangle'
  | 'triangle-down'
  | 'hexagon'
  | 'parallelogram'
  | 'parallelogram-flip'
  | 'cylinder'
  | 'star'
  | 'cloud'

export function boxOf(points: readonly Pt[]): Box {
  if (points.length === 0) return { x: 0, y: 0, w: 0, h: 0 }
  let [minX, minY] = points[0]!
  let [maxX, maxY] = points[0]!
  for (const [x, y] of points) {
    minX = Math.min(minX, x)
    minY = Math.min(minY, y)
    maxX = Math.max(maxX, x)
    maxY = Math.max(maxY, y)
  }
  return { x: minX, y: minY, w: maxX - minX, h: maxY - minY }
}

export function union(boxes: readonly Box[]): Box | null {
  if (boxes.length === 0) return null
  return boxOf(boxes.flatMap((b): Pt[] => [[b.x, b.y], [b.x + b.w, b.y + b.h]]))
}

/** A box with positive width and height from two corners. */
export function boxFrom(a: Pt, b: Pt): Box {
  return { x: Math.min(a[0], b[0]), y: Math.min(a[1], b[1]), w: Math.abs(a[0] - b[0]), h: Math.abs(a[1] - b[1]) }
}

export function grow(b: Box, by: number): Box {
  return { x: b.x - by, y: b.y - by, w: b.w + 2 * by, h: b.h + 2 * by }
}

export function center(b: Box): Pt {
  return [b.x + b.w / 2, b.y + b.h / 2]
}

export function inBox(p: Pt, b: Box): boolean {
  return p[0] >= b.x && p[0] <= b.x + b.w && p[1] >= b.y && p[1] <= b.y + b.h
}

/** Whether a lies completely within b. */
export function contains(b: Box, a: Box): boolean {
  return a.x >= b.x && a.y >= b.y && a.x + a.w <= b.x + b.w && a.y + a.h <= b.y + b.h
}

export function dist(a: Pt, b: Pt): number {
  return Math.hypot(a[0] - b[0], a[1] - b[1])
}

/** Distance of p from the segment a–b. */
export function segmentDist(p: Pt, a: Pt, b: Pt): number {
  const dx = b[0] - a[0]
  const dy = b[1] - a[1]
  const len = dx * dx + dy * dy
  if (len === 0) return dist(p, a)
  const t = Math.max(0, Math.min(1, ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / len))
  return dist(p, [a[0] + t * dx, a[1] + t * dy])
}

/** Distance of p from a polyline. */
export function polylineDist(p: Pt, points: readonly Pt[]): number {
  if (points.length === 1) return dist(p, points[0]!)
  let best = Infinity
  for (let i = 1; i < points.length; i++) best = Math.min(best, segmentDist(p, points[i - 1]!, points[i]!))
  return best
}

/** The corners of a diamond in a box, clockwise from the top. */
export function diamondPoints(b: Box): Pt[] {
  return [
    [b.x + b.w / 2, b.y],
    [b.x + b.w, b.y + b.h / 2],
    [b.x + b.w / 2, b.y + b.h],
    [b.x, b.y + b.h / 2],
  ]
}

/** A triangle with its tip at the top (or at the bottom when down). */
export function trianglePoints(b: Box, down = false): Pt[] {
  const cx = b.x + b.w / 2
  return down
    ? [[b.x, b.y], [b.x + b.w, b.y], [cx, b.y + b.h]]
    : [[cx, b.y], [b.x + b.w, b.y + b.h], [b.x, b.y + b.h]]
}

/** How far the slanted sides of hexagons and parallelograms reach in. */
export const hexagonInset = (b: Box) => Math.min(b.w / 4, b.h / 2)
export const parallelogramSkew = (b: Box) => Math.min(b.w / 4, b.h / 2)

/** A hexagon with corners left and right, flat at the top and bottom. */
export function hexagonPoints(b: Box): Pt[] {
  const i = hexagonInset(b)
  const cy = b.y + b.h / 2
  return [
    [b.x + i, b.y],
    [b.x + b.w - i, b.y],
    [b.x + b.w, cy],
    [b.x + b.w - i, b.y + b.h],
    [b.x + i, b.y + b.h],
    [b.x, cy],
  ]
}

/** A parallelogram leaning right (left when flipped). */
export function parallelogramPoints(b: Box, flip = false): Pt[] {
  const k = parallelogramSkew(b)
  return flip
    ? [[b.x, b.y], [b.x + b.w - k, b.y], [b.x + b.w, b.y + b.h], [b.x + k, b.y + b.h]]
    : [[b.x + k, b.y], [b.x + b.w, b.y], [b.x + b.w - k, b.y + b.h], [b.x, b.y + b.h]]
}

/** Inner corners of the star, relative to the outer ones. */
const STAR_INNER = 0.42

/** A five-pointed star filling the box, one point at the top. */
export function starPoints(b: Box): Pt[] {
  const unit: Pt[] = []
  for (let i = 0; i < 10; i++) {
    const a = -Math.PI / 2 + (i * Math.PI) / 5
    const r = i % 2 === 0 ? 1 : STAR_INNER
    unit.push([Math.cos(a) * r, Math.sin(a) * r])
  }
  // The outer points span ±cos 18° across and -1 … cos 36° down.
  const sx = Math.cos(Math.PI / 10)
  const sy = Math.cos(Math.PI / 5)
  return unit.map(([x, y]) => [b.x + ((x + sx) / (2 * sx)) * b.w, b.y + ((y + 1) / (1 + sy)) * b.h])
}

/** Where the star's middle lies (its points are not symmetric top to bottom). */
export function starCenter(b: Box): Pt {
  return [b.x + b.w / 2, b.y + b.h / (1 + Math.cos(Math.PI / 5))]
}

/** The half height of a cylinder's top and bottom ellipses. */
export const cylinderRy = (b: Box) => Math.min(b.h / 4, b.w * 0.18)

/** Points along half an ellipse: from angle a0 to a1 (radians, y down). */
function arcPoints(cx: number, cy: number, rx: number, ry: number, a0: number, a1: number, n: number): Pt[] {
  const out: Pt[] = []
  for (let i = 0; i <= n; i++) {
    const a = a0 + ((a1 - a0) * i) / n
    out.push([cx + Math.cos(a) * rx, cy + Math.sin(a) * ry])
  }
  return out
}

/** The silhouette of a cylinder: the top and bottom arcs and the sides. */
function cylinderPoints(b: Box): Pt[] {
  const ry = cylinderRy(b)
  const rx = b.w / 2
  const cx = b.x + rx
  return [...arcPoints(cx, b.y + ry, rx, ry, Math.PI, 2 * Math.PI, 12), ...arcPoints(cx, b.y + b.h - ry, rx, ry, 0, Math.PI, 12)]
}

/** One bump of a cloud: an elliptical arc between two dips, clockwise. */
export interface CloudArc {
  from: Pt
  to: Pt
  rx: number
  ry: number
  /** The arc is more than half its ellipse. */
  large: boolean
  /** Its middle and the angles it runs through (for points along it). */
  cx: number
  cy: number
  a0: number
  a1: number
}

// The bumps of a cloud by the angles of the dips between them (degrees,
// clockwise from the right): larger bumps on top, smaller ones below.
const CLOUD_DIPS = [165, 215, 270, 330, 375, 420, 465]
const CLOUD_DEPTH = 0.76

interface UnitArc {
  from: Pt
  to: Pt
  c: Pt
  r: number
  a0: number
  a1: number
}

const unitCloud: { arcs: UnitArc[]; box: Box } = (() => {
  const rad = (d: number) => (d * Math.PI) / 180
  const arcs: UnitArc[] = []
  for (let i = 0; i < CLOUD_DIPS.length; i++) {
    const d0 = rad(CLOUD_DIPS[i]!)
    const d1 = rad(CLOUD_DIPS[(i + 1) % CLOUD_DIPS.length]! + (i + 1 === CLOUD_DIPS.length ? 360 : 0))
    const half = (d1 - d0) / 2
    const mid = d0 + half
    const from: Pt = [Math.cos(d0) * CLOUD_DEPTH, Math.sin(d0) * CLOUD_DEPTH]
    const to: Pt = [Math.cos(d1) * CLOUD_DEPTH, Math.sin(d1) * CLOUD_DEPTH]
    // A circle through both dips whose top reaches radius 1 at the middle.
    const chord = CLOUD_DEPTH * Math.sin(half)
    const sag = 1 - CLOUD_DEPTH * Math.cos(half)
    const r = (chord * chord + sag * sag) / (2 * sag)
    const c: Pt = [Math.cos(mid) * (1 - r), Math.sin(mid) * (1 - r)]
    const a0 = Math.atan2(from[1] - c[1], from[0] - c[0])
    let a1 = Math.atan2(to[1] - c[1], to[0] - c[0])
    while (a1 <= a0) a1 += 2 * Math.PI
    arcs.push({ from, to, c, r, a0, a1 })
  }
  const pts = arcs.flatMap((a) => arcPoints(a.c[0], a.c[1], a.r, a.r, a.a0, a.a1, 24))
  return { arcs, box: boxOf(pts) }
})()

/** The bumps of a cloud filling the box. */
export function cloudArcs(b: Box): CloudArc[] {
  const u = unitCloud.box
  const sx = b.w / u.w
  const sy = b.h / u.h
  const map = (p: Pt): Pt => [b.x + (p[0] - u.x) * sx, b.y + (p[1] - u.y) * sy]
  return unitCloud.arcs.map((a) => {
    const [cx, cy] = map(a.c)
    return { from: map(a.from), to: map(a.to), rx: a.r * sx, ry: a.r * sy, large: a.a1 - a.a0 > Math.PI, cx, cy, a0: a.a0, a1: a.a1 }
  })
}

function cloudPoints(b: Box): Pt[] {
  return cloudArcs(b).flatMap((a) => arcPoints(a.cx, a.cy, a.rx, a.ry, a.a0, a.a1, 8).slice(1))
}

/** The outline as a polygon, or null for rectangles and ellipses. */
export function outlinePolygon(b: Box, shape: Outline): Pt[] | null {
  switch (shape) {
    case 'rect':
    case 'ellipse':
      return null
    case 'diamond':
      return diamondPoints(b)
    case 'triangle':
      return trianglePoints(b)
    case 'triangle-down':
      return trianglePoints(b, true)
    case 'hexagon':
      return hexagonPoints(b)
    case 'parallelogram':
      return parallelogramPoints(b)
    case 'parallelogram-flip':
      return parallelogramPoints(b, true)
    case 'cylinder':
      return cylinderPoints(b)
    case 'star':
      return starPoints(b)
    case 'cloud':
      return cloudPoints(b)
  }
}

/** Whether p lies inside a polygon (even-odd rule). */
export function insidePolygon(p: Pt, poly: readonly Pt[]): boolean {
  let inside = false
  for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
    const [xi, yi] = poly[i]!
    const [xj, yj] = poly[j]!
    if (yi > p[1] !== yj > p[1] && p[0] < ((xj - xi) * (p[1] - yi)) / (yj - yi) + xi) inside = !inside
  }
  return inside
}

/** Whether p lies inside the outline. */
export function insideOutline(p: Pt, b: Box, shape: Outline): boolean {
  if (b.w <= 0 || b.h <= 0) return false
  const [cx, cy] = center(b)
  const nx = (p[0] - cx) / (b.w / 2)
  const ny = (p[1] - cy) / (b.h / 2)
  switch (shape) {
    case 'rect':
      return Math.abs(nx) <= 1 && Math.abs(ny) <= 1
    case 'ellipse':
      return nx * nx + ny * ny <= 1
    default:
      return insidePolygon(p, outlinePolygon(b, shape)!)
  }
}

/** Distance of p from the outline (approximate for ellipses). */
export function outlineDist(p: Pt, b: Box, shape: Outline): number {
  switch (shape) {
    case 'rect':
      return polylineDist(p, [[b.x, b.y], [b.x + b.w, b.y], [b.x + b.w, b.y + b.h], [b.x, b.y + b.h], [b.x, b.y]])
    case 'ellipse': {
      const [cx, cy] = center(b)
      const angle = Math.atan2((p[1] - cy) / Math.max(b.h, 1e-9), (p[0] - cx) / Math.max(b.w, 1e-9))
      return dist(p, [cx + (Math.cos(angle) * b.w) / 2, cy + (Math.sin(angle) * b.h) / 2])
    }
    default: {
      const poly = outlinePolygon(b, shape)!
      return polylineDist(p, [...poly, poly[0]!])
    }
  }
}

/** The farthest distance along the ray o + t·d at which it crosses the polygon. */
function rayExit(o: Pt, d: Pt, poly: readonly Pt[]): number | null {
  let best: number | null = null
  for (let i = 0; i < poly.length; i++) {
    const a = poly[i]!
    const b = poly[(i + 1) % poly.length]!
    const ex = b[0] - a[0]
    const ey = b[1] - a[1]
    const den = d[0] * ey - d[1] * ex
    if (Math.abs(den) < 1e-12) continue
    const ax = a[0] - o[0]
    const ay = a[1] - o[1]
    const t = (ax * ey - ay * ex) / den
    const s = (ax * d[1] - ay * d[0]) / den
    if (t >= 0 && s >= -1e-9 && s <= 1 + 1e-9 && (best === null || t > best)) best = t
  }
  return best
}

/**
 * Where the ray from the center of the box toward `toward` leaves the
 * outline, pushed out by gap. A point at the center gives the top.
 */
export function outlinePoint(b: Box, shape: Outline, toward: Pt, gap = 0): Pt {
  const [cx, cy] = center(b)
  let dx = toward[0] - cx
  let dy = toward[1] - cy
  const len = Math.hypot(dx, dy)
  if (len < 1e-9) {
    dx = 0
    dy = -1
  } else {
    dx /= len
    dy /= len
  }
  const a = Math.max(b.w / 2, 1e-9)
  const c = Math.max(b.h / 2, 1e-9)
  const rectT = Math.min(dx === 0 ? Infinity : a / Math.abs(dx), dy === 0 ? Infinity : c / Math.abs(dy))
  let t: number
  switch (shape) {
    case 'rect':
      t = rectT
      break
    case 'ellipse':
      t = 1 / Math.sqrt((dx * dx) / (a * a) + (dy * dy) / (c * c))
      break
    case 'diamond':
      t = 1 / (Math.abs(dx) / a + Math.abs(dy) / c)
      break
    default:
      t = rectT
      if (b.w > 0 && b.h > 0) t = rayExit([cx, cy], [dx, dy], outlinePolygon(b, shape)!) ?? rectT
  }
  return [cx + dx * (t + gap), cy + dy * (t + gap)]
}

export type Side = 'top' | 'right' | 'bottom' | 'left'

/** The side of the box that faces p (relative to its proportions). */
export function facingSide(b: Box, p: Pt): Side {
  const [cx, cy] = center(b)
  const nx = (p[0] - cx) / Math.max(b.w, 1)
  const ny = (p[1] - cy) / Math.max(b.h, 1)
  if (Math.abs(nx) >= Math.abs(ny)) return nx >= 0 ? 'right' : 'left'
  return ny >= 0 ? 'bottom' : 'top'
}

/** The middle of a side, pushed out by gap. */
export function sidePoint(b: Box, side: Side, gap = 0): Pt {
  const [cx, cy] = center(b)
  switch (side) {
    case 'top':
      return [cx, b.y - gap]
    case 'bottom':
      return [cx, b.y + b.h + gap]
    case 'left':
      return [b.x - gap, cy]
    case 'right':
      return [b.x + b.w + gap, cy]
  }
}

export const horizontal = (s: Side) => s === 'left' || s === 'right'

/** Snaps a value to the grid. */
export function snap(v: number, grid: number): number {
  return Math.round(v / grid) * grid
}

/** Rounds to two decimals, which keeps saved files short. */
export function round(v: number): number {
  return Math.round(v * 100) / 100
}

/** Fewer points along nearly straight stretches (Ramer–Douglas–Peucker). */
export function simplify(points: readonly Pt[], tolerance: number): Pt[] {
  if (points.length < 3) return [...points]
  const keep = new Uint8Array(points.length)
  keep[0] = keep[points.length - 1] = 1
  const stack: [number, number][] = [[0, points.length - 1]]
  while (stack.length) {
    const [a, b] = stack.pop()!
    let worst = -1
    let index = -1
    for (let i = a + 1; i < b; i++) {
      const d = segmentDist(points[i]!, points[a]!, points[b]!)
      if (d > worst) {
        worst = d
        index = i
      }
    }
    if (worst > tolerance) {
      keep[index] = 1
      stack.push([a, index], [index, b])
    }
  }
  return points.filter((_, i) => keep[i])
}
