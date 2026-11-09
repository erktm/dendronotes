// Sketchy strokes: every line drawn twice with a slight wobble, the same way
// each time for the same seed, so a drawing does not change when nothing in
// it changed.

import type { Pt } from './geometry'
import { round } from './geometry'

/** A small, seeded random number generator (mulberry32), giving 0 … 1. */
export function random(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const f = (n: number) => String(round(n))

/** One wobbly stroke from a to b as a path segment (starting with M). */
function wobble(a: Pt, b: Pt, rnd: () => number, amount: number): string {
  const len = Math.hypot(b[0] - a[0], b[1] - a[1])
  const off = Math.min(amount, len / 10 + 0.5)
  const j = () => (rnd() - 0.5) * 2 * off
  const bow = (rnd() - 0.5) * Math.min(len / 12, amount * 2)
  // The control point sits near the middle, pushed sideways.
  const nx = len > 0 ? -(b[1] - a[1]) / len : 0
  const ny = len > 0 ? (b[0] - a[0]) / len : 0
  const mx = (a[0] + b[0]) / 2 + nx * bow + j() / 2
  const my = (a[1] + b[1]) / 2 + ny * bow + j() / 2
  return `M${f(a[0] + j())} ${f(a[1] + j())}Q${f(mx)} ${f(my)} ${f(b[0] + j())} ${f(b[1] + j())}`
}

/** A sketchy polyline (closed when closed is set), drawn twice. */
export function sketchLines(points: readonly Pt[], seed: number, closed = false, amount = 2): string {
  const rnd = random(seed)
  const pts = closed ? [...points, points[0]!] : points
  let d = ''
  for (let pass = 0; pass < 2; pass++) {
    for (let i = 1; i < pts.length; i++) d += wobble(pts[i - 1]!, pts[i]!, rnd, amount)
  }
  return d
}

/** A sketchy ellipse, drawn twice as a slightly irregular closed curve. */
export function sketchEllipse(cx: number, cy: number, rx: number, ry: number, seed: number, amount = 2): string {
  const rnd = random(seed)
  let d = ''
  const n = 12
  for (let pass = 0; pass < 2; pass++) {
    const start = rnd() * Math.PI * 2
    const pts: Pt[] = []
    // A little more than a full turn, so the ends overlap like a pen stroke.
    for (let i = 0; i <= n + 1; i++) {
      const t = start + (i / n) * Math.PI * 2
      const k = 1 + (rnd() - 0.5) * (amount / Math.max(8, Math.min(rx, ry)))
      pts.push([cx + Math.cos(t) * rx * k, cy + Math.sin(t) * ry * k])
    }
    d += smooth(pts)
  }
  return d
}

/** A smooth path through points (Catmull-Rom as cubic Béziers). */
export function smooth(points: readonly Pt[]): string {
  if (points.length === 0) return ''
  if (points.length < 3) return `M${points.map((p) => `${f(p[0])} ${f(p[1])}`).join('L')}`
  let d = `M${f(points[0]![0])} ${f(points[0]![1])}`
  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[Math.max(0, i - 1)]!
    const p1 = points[i]!
    const p2 = points[i + 1]!
    const p3 = points[Math.min(points.length - 1, i + 2)]!
    const c1: Pt = [p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6]
    const c2: Pt = [p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6]
    d += `C${f(c1[0])} ${f(c1[1])} ${f(c2[0])} ${f(c2[1])} ${f(p2[0])} ${f(p2[1])}`
  }
  return d
}

/** A sketchy rectangle with rounded corners, drawn twice. */
export function sketchRoundRect(x: number, y: number, w: number, h: number, r: number, seed: number, amount = 2): string {
  const rnd = random(seed)
  // Each corner wobbles as a whole, so it stays round; the sides between
  // them take up the difference.
  const corner = (cx: number, cy: number, from: number): Pt[] => {
    const dx = (rnd() - 0.5) * amount
    const dy = (rnd() - 0.5) * amount
    return [0, 0.5, 1].map((k) => {
      const t = from + (k * Math.PI) / 2
      return [cx + Math.cos(t) * r + dx, cy + Math.sin(t) * r + dy]
    })
  }
  let d = ''
  for (let pass = 0; pass < 2; pass++) {
    // Clockwise from the top right corner, past the start so the ends overlap.
    const pts: Pt[] = [
      ...corner(x + w - r, y + r, -Math.PI / 2),
      ...corner(x + w - r, y + h - r, 0),
      ...corner(x + r, y + h - r, Math.PI / 2),
      ...corner(x + r, y + r, Math.PI),
    ]
    d += smooth([...pts, pts[0]!, pts[1]!])
  }
  return d
}
