// Plane geometry for drawings: boxes, distances and where a line meets the
// outline of a shape.

export type Pt = [number, number]

export interface Box {
  x: number
  y: number
  w: number
  h: number
}

export type Outline = 'rect' | 'ellipse' | 'diamond'

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
    case 'diamond':
      return Math.abs(nx) + Math.abs(ny) <= 1
  }
}

/** Distance of p from the outline (approximate for ellipses). */
export function outlineDist(p: Pt, b: Box, shape: Outline): number {
  switch (shape) {
    case 'rect':
      return polylineDist(p, [[b.x, b.y], [b.x + b.w, b.y], [b.x + b.w, b.y + b.h], [b.x, b.y + b.h], [b.x, b.y]])
    case 'diamond': {
      const d = diamondPoints(b)
      return polylineDist(p, [...d, d[0]!])
    }
    case 'ellipse': {
      const [cx, cy] = center(b)
      const angle = Math.atan2((p[1] - cy) / Math.max(b.h, 1e-9), (p[0] - cx) / Math.max(b.w, 1e-9))
      return dist(p, [cx + (Math.cos(angle) * b.w) / 2, cy + (Math.sin(angle) * b.h) / 2])
    }
  }
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
  let t: number
  switch (shape) {
    case 'rect':
      t = Math.min(dx === 0 ? Infinity : a / Math.abs(dx), dy === 0 ? Infinity : c / Math.abs(dy))
      break
    case 'ellipse':
      t = 1 / Math.sqrt((dx * dx) / (a * a) + (dy * dy) / (c * c))
      break
    case 'diamond':
      t = 1 / (Math.abs(dx) / a + Math.abs(dy) / c)
      break
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
