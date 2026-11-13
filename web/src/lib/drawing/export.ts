// Downloading a drawing: the SVG file itself (it stays editable in
// dendronotes), or a PNG picture of it in light or dark.

import type { Scheme } from './colors'
import { measureText } from './dom'
import type { Scene } from './scene'
import { canvasBox, toSVG } from './svg'
import { saveFile } from '../download'

export { saveFile }

/** Largest side of an exported picture in pixels. */
const MAX_PNG = 8000


export function svgBlob(scene: Scene): Blob {
  return new Blob([toSVG(scene, measureText())], { type: 'image/svg+xml' })
}

/** The drawing as a PNG picture, at scale (2: sharp on high-density screens). */
export async function pngBlob(scene: Scene, scheme: Exclude<Scheme, 'both'>, scale = 2): Promise<Blob> {
  const measure = measureText()
  const box = canvasBox(scene, measure)
  const k = Math.min(scale, MAX_PNG / Math.max(box.w, box.h))
  const url = URL.createObjectURL(new Blob([toSVG(scene, measure, scheme)], { type: 'image/svg+xml' }))
  try {
    const img = new Image()
    img.src = url
    await img.decode()
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(box.w * k))
    canvas.height = Math.max(1, Math.round(box.h * k))
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('no canvas')
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height)
    return await new Promise<Blob>((resolve, reject) =>
      canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('encoding failed'))), 'image/png'),
    )
  } finally {
    URL.revokeObjectURL(url)
  }
}
