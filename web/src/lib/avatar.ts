// Avatars: a designed picture (color, pattern, initials) or an uploaded
// photo. Colors are the eight peer colors (tokens --peer-0 … --peer-7), so
// they work in every palette, light and dark.

import type { Avatar, AvatarPattern } from './api/soma'

export const AVATAR_COLORS = 8
export const AVATAR_PATTERNS: readonly AvatarPattern[] = ['plain', 'dendrite', 'rings', 'vesicles']

/** One letter or two, from a name: "Erik Häger" → "EH", "anna" → "AN". */
export function initials(name: string): string {
  const parts = name.trim().split(/[\s._@-]+/).filter(Boolean)
  const letters = parts.length > 1 ? [[...parts[0]!][0], [...parts[1]!][0]] : [...(parts[0] ?? '?')].slice(0, 2)
  return letters.join('').toUpperCase()
}

/** What the server accepts as chosen initials: up to two letters or digits. */
export function cleanInitials(value: string): string {
  return [...value.trim().toUpperCase()].filter((c) => /[\p{L}\p{N}]/u.test(c)).slice(0, 2).join('')
}

/** The URL of a photo, or null when the avatar has none. */
export function photoURL(userID: number, avatar: Avatar | undefined): string | null {
  return avatar && avatar.photo > 0 ? `/api/users/${userID}/avatar?v=${avatar.photo}` : null
}

/** An avatar for someone whose design is unknown (a color from the id). */
export function fallbackAvatar(userID: number): Avatar {
  return { color: ((userID % AVATAR_COLORS) + AVATAR_COLORS) % AVATAR_COLORS, pattern: 'plain', initials: '', photo: 0 }
}

/** Crops the square at (x, y) with side length `side` out of an image into a size×size PNG. */
export async function cropToPNG(
  image: CanvasImageSource,
  crop: { x: number; y: number; side: number },
  size = 256,
): Promise<Blob> {
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = size
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas unavailable')
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(image, crop.x, crop.y, crop.side, crop.side, 0, 0, size, size)
  return new Promise((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('encoding failed'))), 'image/png'),
  )
}
