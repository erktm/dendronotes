// Pictures for drawings: icons from the library become data:image URLs (a
// drawing keeps its own copy), and uploads other than SVG become PNG of at
// most 512 pixels, which the server encodes anew.

import { DATA_IMAGE, type FileData } from './scene'

export const MAX_ICON_SIZE = 512

export function toDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result))
    r.onerror = () => reject(r.error ?? new Error('reading failed'))
    r.readAsDataURL(blob)
  })
}

/** The natural size of a picture; SVG without a size counts as 96 × 96. */
export function pictureSize(src: string): Promise<{ w: number; h: number }> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => resolve({ w: img.naturalWidth || 96, h: img.naturalHeight || 96 })
    img.onerror = () => resolve({ w: 96, h: 96 })
    img.src = src
  })
}

/** A picture from a URL of this app (an icon) as a drawing's own file. */
export async function pictureFile(url: string): Promise<FileData | null> {
  const res = await fetch(url, { credentials: 'same-origin' })
  if (!res.ok) return null
  const blob = await res.blob()
  const type = blob.type === 'image/svg+xml' ? 'image/svg+xml' : 'image/png'
  const src = await toDataURL(new Blob([blob], { type }))
  if (!DATA_IMAGE.test(src)) return null
  const { w, h } = await pictureSize(src)
  return { src, w, h }
}

/** An uploaded picture ready for the library: SVG as is, everything else as PNG of at most 512 px. */
export async function iconUpload(file: File): Promise<Blob | null> {
  if (file.type === 'image/svg+xml' || file.name.toLowerCase().endsWith('.svg')) {
    return new Blob([await file.arrayBuffer()], { type: 'image/svg+xml' })
  }
  let bitmap: ImageBitmap
  try {
    bitmap = await createImageBitmap(file)
  } catch {
    return null
  }
  const scale = Math.min(1, MAX_ICON_SIZE / Math.max(bitmap.width, bitmap.height))
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(bitmap.width * scale))
  canvas.height = Math.max(1, Math.round(bitmap.height * scale))
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
  bitmap.close()
  return new Promise((resolve) => canvas.toBlob((b) => resolve(b), 'image/png'))
}

/** "Oak tree" for "Oak tree.png". */
export function iconName(file: File): string {
  const name = file.name.replace(/\.[^.]+$/, '').trim() || 'Icon'
  return [...name].slice(0, 80).join('')
}
