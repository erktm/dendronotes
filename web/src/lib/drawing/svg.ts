// The file of a drawing ("Name.drawing.svg"): plain SVG any viewer shows,
// with its own style sheet for light and dark, the pictures once each as
// <symbol>, and the scene as JSON in <metadata id="dendronotes-drawing">.
// The server checks every saved file (internal/drawing); what is written
// here must pass that check (testdata/drawing.svg is shared with its tests).

import { PAPER_CLASS, styleSheet } from './colors'
import { type Box, grow, union } from './geometry'
import { type Measure, type Prim, lineLabelBox, primitives, usedColors } from './render'
import { type FileData, type Scene, bounds, isLinear, pruneFiles, sanitizeScene } from './scene'

export const METADATA_ID = 'dendronotes-drawing'
const SVG_NS = 'http://www.w3.org/2000/svg'
const XLINK_NS = 'http://www.w3.org/1999/xlink'

/** Space around the drawing in a saved file. */
const MARGIN = 16

const escapeText = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
const escapeAttr = (s: string) => escapeText(s).replace(/"/g, '&quot;')

function attrs(a: Record<string, string>): string {
  return Object.entries(a)
    .map(([k, v]) => ` ${k}="${escapeAttr(v)}"`)
    .join('')
}

function primMarkup(p: Prim): string {
  if (p.tag === 'text') {
    const spans = (p.lines ?? []).map((l) => `<tspan x="${l.x}" y="${l.y}">${escapeText(l.text)}</tspan>`).join('')
    return `<text xml:space="preserve"${attrs(p.attrs)}>${spans}</text>`
  }
  if (p.tag === 'image') return `<use href="#file-${escapeAttr(p.file ?? '')}"${attrs(p.attrs)}/>`
  return `<${p.tag}${attrs(p.attrs)}/>`
}

/** The area a drawing covers, labels and strokes included. */
export function drawingBox(scene: Scene, measure: Measure): Box | null {
  const boxes = scene.elements.flatMap((e) => {
    const b = grow(bounds(e, scene), e.width + (e.type === 'arrow' || e.type === 'line' ? 12 : 0))
    const label = isLinear(e) ? lineLabelBox(e, scene, measure) : null
    return label ? [b, label] : [b]
  })
  return union(boxes)
}

/** The canvas of a saved drawing: what it covers and a margin, in whole units (400 × 300 when empty). */
export function canvasBox(scene: Scene, measure: Measure): Box {
  const box = drawingBox(scene, measure)
  if (!box) return { x: 0, y: 0, w: 400, h: 300 }
  const x = Math.floor(box.x - MARGIN)
  const y = Math.floor(box.y - MARGIN)
  return { x, y, w: Math.ceil(box.x + box.w + MARGIN) - x, h: Math.ceil(box.y + box.h + MARGIN) - y }
}

/** The file of a drawing. */
export function toSVG(input: Scene, measure: Measure): string {
  const scene = pruneFiles(input)
  const { x, y, w, h } = canvasBox(scene, measure)
  const json = JSON.stringify({ version: 1, sketchy: scene.sketchy, elements: scene.elements })
  const defs = Object.entries(scene.files)
    .map(
      ([key, file]) =>
        `<symbol id="file-${escapeAttr(key)}" viewBox="0 0 ${file.w} ${file.h}">` +
        `<image href="${escapeAttr(file.src)}" width="${file.w}" height="${file.h}"/></symbol>`,
    )
    .join('')
  const body = scene.elements.map((e) => `<g>${primitives(e, scene, measure).map(primMarkup).join('')}</g>`).join('\n')
  // Its own page color, switching with the colors: other viewers may show
  // dark colors on a white page or the other way round.
  const paper = `<rect class="${PAPER_CLASS}" x="${x}" y="${y}" width="${w}" height="${h}"/>`
  return (
    `<svg xmlns="${SVG_NS}" viewBox="${x} ${y} ${w} ${h}" width="${w}" height="${h}">\n` +
    `<style>${styleSheet(usedColors(scene))}</style>\n` +
    `<metadata id="${METADATA_ID}">${escapeText(json)}</metadata>\n` +
    (defs ? `<defs>${defs}</defs>\n` : '') +
    paper +
    '\n' +
    body +
    (body ? '\n' : '') +
    `</svg>\n`
  )
}

/** The scene of a drawing file, or null when it has none (or a broken one). Needs a DOM. */
export function parseDrawing(svg: string): Scene | null {
  const doc = new DOMParser().parseFromString(svg, 'image/svg+xml')
  if (doc.getElementsByTagName('parsererror').length > 0 || doc.documentElement.localName !== 'svg') return null
  const metas = [...doc.getElementsByTagNameNS(SVG_NS, 'metadata')]
  const meta = metas.find((m) => m.getAttribute('id') === METADATA_ID) ?? metas[0]
  if (!meta) return null
  let raw: unknown
  try {
    raw = JSON.parse(meta.textContent ?? '')
  } catch {
    return null
  }
  const files: Record<string, FileData> = {}
  for (const sym of doc.getElementsByTagNameNS(SVG_NS, 'symbol')) {
    const id = sym.getAttribute('id') ?? ''
    const img = sym.getElementsByTagNameNS(SVG_NS, 'image')[0]
    if (!id.startsWith('file-') || !img) continue
    const src = img.getAttribute('href') ?? img.getAttributeNS(XLINK_NS, 'href') ?? ''
    const [, , vw, vh] = (sym.getAttribute('viewBox') ?? '').trim().split(/[\s,]+/).map(Number)
    files[id.slice('file-'.length)] = { src, w: vw || 1, h: vh || 1 }
  }
  return sanitizeScene(raw, files)
}
