// Attachments: how files in the soma are shown and where they are served.
// The server decides what may be uploaded (internal/attach); this mirrors
// its type list for display.

import { encodePath, extname } from './paths'
import { somaBase } from './api/base'

export type FileKind = 'image' | 'audio' | 'video' | 'pdf' | 'text' | 'other'

const KINDS: Record<string, FileKind> = {
  '.png': 'image', '.jpg': 'image', '.jpeg': 'image', '.gif': 'image', '.webp': 'image',
  '.avif': 'image', '.bmp': 'image', '.ico': 'image', '.svg': 'image',
  '.mp3': 'audio', '.wav': 'audio', '.ogg': 'audio', '.flac': 'audio', '.m4a': 'audio',
  '.mp4': 'video', '.webm': 'video', '.mov': 'video',
  '.pdf': 'pdf',
  '.md': 'text', '.markdown': 'text', '.txt': 'text', '.csv': 'text', '.json': 'text',
}

/** How a file is shown (by its extension). */
export function fileKind(path: string): FileKind {
  return KINDS[extname(path)] ?? 'other'
}

/** Extensions the server accepts for upload. */
export const UPLOAD_EXTENSIONS = [...Object.keys(KINDS), '.zip']

/** URL of a file's content in the current soma (same origin, with the session cookie). */
export function fileURL(path: string): string {
  return `${somaBase()}/files/${encodePath(path)}`
}

const FILE_URL = /^\/api\/somas\/[a-z0-9]{1,40}\/files\/[^\\]*$/

/** Whether a URL points to a file served from a soma (and nothing else). */
export function isFileURL(src: string): boolean {
  return FILE_URL.test(src)
}

/** "1.4 MB" */
export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB']
  let v = bytes / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`
}

/** Name for a pasted image without a file name, like "Pasted image 20261106-081203.png". */
export function pastedName(type: string, now = new Date()): string {
  const ext = { 'image/png': '.png', 'image/jpeg': '.jpg', 'image/gif': '.gif', 'image/webp': '.webp' }[type] ?? '.png'
  const p = (n: number) => String(n).padStart(2, '0')
  const stamp = `${now.getFullYear()}${p(now.getMonth() + 1)}${p(now.getDate())}-${p(now.getHours())}${p(now.getMinutes())}${p(now.getSeconds())}`
  return `Pasted image ${stamp}${ext}`
}
