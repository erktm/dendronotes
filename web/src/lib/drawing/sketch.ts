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

/**
 * One hand-drawn stroke from a to b: a gently bowed curve whose ends may run
 * a little past the corner (over) or stop just short of it, like a pen.
 * k scales the wobble (the second pass retraces with less).
 */
function stroke(a: Pt, b: Pt, rnd: () => number, amount: number, k: number, overA: boolean, overB: boolean): string {
  const len = Math.hypot(b[0] - a[0], b[1] - a[1])
  if (len < 0.5) return `M${f(a[0])} ${f(a[1])}L${f(b[0])} ${f(b[1])}`
  const ux = (b[0] - a[0]) / len
  const uy = (b[1] - a[1]) / len
  const nx = -uy
  const ny = ux
  const s = (n: number) => (rnd() - 0.5) * 2 * n * k
  // How far the stroke drifts across, and how much it bows: more for long lines, within limits.
  const drift = Math.min(amount * 0.8, len / 20 + 0.4)
  const bow = s(Math.min(amount * 2.2, len * 0.02 + 0.8))
  const over = Math.min(amount * 2, len * 0.04)
  const extA = overA ? over * (rnd() * 1.4 - 0.4) : 0
  const extB = overB ? over * (rnd() * 1.4 - 0.4) : 0
  const pa: Pt = [a[0] - ux * extA + nx * s(drift), a[1] - uy * extA + ny * s(drift)]
  const pb: Pt = [b[0] + ux * extB + nx * s(drift), b[1] + uy * extB + ny * s(drift)]
  // The control points share the bow, each with a little of its own.
  const c1 = len * (0.28 + rnd() * 0.1)
  const c2 = len * (0.62 + rnd() * 0.1)
  const b1 = bow + s(drift * 0.5)
  const b2 = bow + s(drift * 0.5)
  return (
    `M${f(pa[0])} ${f(pa[1])}` +
    `C${f(a[0] + ux * c1 + nx * b1)} ${f(a[1] + uy * c1 + ny * b1)} ` +
    `${f(a[0] + ux * c2 + nx * b2)} ${f(a[1] + uy * c2 + ny * b2)} ${f(pb[0])} ${f(pb[1])}`
  )
}

/**
 * A sketchy polyline (closed when closed is set), drawn twice. Corners get
 * the small overshoots of a pen; the open ends of a line stay where they are
 * (an arrowhead sits there).
 */
export function sketchLines(points: readonly Pt[], seed: number, closed = false, amount = 2): string {
  const rnd = random(seed)
  const pts = closed ? [...points, points[0]!] : points
  let d = ''
  for (let pass = 0; pass < 2; pass++) {
    const k = pass === 0 ? 1 : 0.8
    for (let i = 1; i < pts.length; i++) {
      d += stroke(pts[i - 1]!, pts[i]!, rnd, amount, k, closed || i > 1, closed || i < pts.length - 1)
    }
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

/** A sketchy part of an ellipse from angle a0 to a1 (radians, y down), drawn twice. */
export function sketchArc(cx: number, cy: number, rx: number, ry: number, a0: number, a1: number, seed: number, amount = 2): string {
  const rnd = random(seed)
  const n = Math.max(3, Math.ceil(Math.abs(a1 - a0) / (Math.PI / 6)))
  let d = ''
  for (let pass = 0; pass < 2; pass++) {
    const pts: Pt[] = []
    for (let i = 0; i <= n; i++) {
      const t = a0 + ((a1 - a0) * i) / n
      const k = 1 + (rnd() - 0.5) * (amount / Math.max(8, Math.min(rx, ry)))
      pts.push([cx + Math.cos(t) * rx * k, cy + Math.sin(t) * ry * k])
    }
    d += smooth(pts)
  }
  return d
}

/** A bump of a cloud: part of an ellipse from angle a0 to a1 (radians, y down). */
export interface Bump {
  cx: number
  cy: number
  rx: number
  ry: number
  a0: number
  a1: number
}

/**
 * Sketchy bumps (a cloud), drawn twice: each bump its own slightly uneven
 * stroke that runs a little past the dips, so they meet like pen strokes.
 */
export function sketchBumps(bumps: readonly Bump[], seed: number, amount = 2): string {
  const rnd = random(seed)
  let d = ''
  for (let pass = 0; pass < 2; pass++) {
    for (const b of bumps) {
      const over = (b.a1 - b.a0) * 0.06
      const a0 = b.a0 - over * rnd()
      const a1 = b.a1 + over * rnd()
      const pts: Pt[] = []
      const n = 6
      for (let i = 0; i <= n; i++) {
        const t = a0 + ((a1 - a0) * i) / n
        const k = 1 + (rnd() - 0.5) * (amount / Math.max(8, Math.min(b.rx, b.ry)))
        pts.push([b.cx + Math.cos(t) * b.rx * k, b.cy + Math.sin(t) * b.ry * k])
      }
      d += smooth(pts)
    }
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

/** Circle quarter as a cubic Bézier: control arm length per radius. */
const KAPPA = 0.5523

/**
 * A sketchy rectangle with rounded corners: one continuous stroke around,
 * drawn twice. Each side bows a little, the corners flow into the sides, and
 * the stroke ends a bit past where it began.
 */
export function sketchRoundRect(x: number, y: number, w: number, h: number, r: number, seed: number, amount = 2): string {
  const rnd = random(seed)
  // The sides, clockwise from the top: start, end, direction.
  const sides: [Pt, Pt, Pt][] = [
    [[x + r, y], [x + w - r, y], [1, 0]],
    [[x + w, y + r], [x + w, y + h - r], [0, 1]],
    [[x + w - r, y + h], [x + r, y + h], [-1, 0]],
    [[x, y + h - r], [x, y + r], [0, -1]],
  ]
  let d = ''
  for (let pass = 0; pass < 2; pass++) {
    const k = pass === 0 ? 1 : 0.8
    const s = (n: number) => (rnd() - 0.5) * 2 * n * k
    // Every side shifted across a little at each end.
    const ends = sides.map(([a, b, [ux, uy]]) => {
      const len = Math.hypot(b[0] - a[0], b[1] - a[1])
      const drift = Math.min(amount * 0.8, len / 20 + 0.4)
      const nx = -uy
      const ny = ux
      const sa = s(drift)
      const sb = s(drift)
      return {
        a: [a[0] + nx * sa, a[1] + ny * sa] as Pt,
        b: [b[0] + nx * sb, b[1] + ny * sb] as Pt,
        u: [ux, uy] as Pt,
        n: [nx, ny] as Pt,
        len,
        bow: s(Math.min(amount * 2.2, len * 0.02 + 0.8)),
      }
    })
    const first = ends[0]!
    d += `M${f(first.a[0])} ${f(first.a[1])}`
    for (let i = 0; i < 4; i++) {
      const e = ends[i]!
      const c1 = e.len * 0.33
      const c2 = e.len * 0.67
      d +=
        `C${f(e.a[0] + e.u[0] * c1 + e.n[0] * e.bow)} ${f(e.a[1] + e.u[1] * c1 + e.n[1] * e.bow)} ` +
        `${f(e.a[0] + e.u[0] * c2 + e.n[0] * e.bow)} ${f(e.a[1] + e.u[1] * c2 + e.n[1] * e.bow)} ${f(e.b[0])} ${f(e.b[1])}`
      // The corner: a quarter circle from this side's end into the next side's start.
      const next = ends[(i + 1) % 4]!
      const arm = r * KAPPA * (1 + s(0.12))
      d +=
        `C${f(e.b[0] + e.u[0] * arm)} ${f(e.b[1] + e.u[1] * arm)} ` +
        `${f(next.a[0] - next.u[0] * arm)} ${f(next.a[1] - next.u[1] * arm)} ${f(next.a[0])} ${f(next.a[1])}`
    }
    // Run on a little along the top, like a pen that does not stop exactly.
    const run = Math.min(first.len * 0.3, 4 + rnd() * 10)
    d += `L${f(first.a[0] + first.u[0] * run + first.n[0] * s(1))} ${f(first.a[1] + first.u[1] * run + first.n[1] * s(1))}`
  }
  return d
}
