// Drawings embedded in notes (reading view and live preview): drawn inline
// from their scene, so they take the app's palette, light or dark. Until
// the scene is there – or when the file has none – the plain image shows.

import { versionedFileURL } from '../files'
import { SVG_NS, elementNode, measureText } from './dom'
import type { Scene } from './scene'
import { canvasBox, parseDrawing } from './svg'

const scenes = new Map<string, Promise<Scene | null>>()

/** The scene of a drawing in a version (the file's mtime); loaded once per version. */
export function loadScene(path: string, version: number | undefined): Promise<Scene | null> {
  const key = `${path}\u0000${version ?? ''}`
  let p = scenes.get(key)
  if (!p) {
    p = fetch(versionedFileURL(path, version), { credentials: 'same-origin' })
      .then((r) => (r.ok ? r.text() : ''))
      .then((text) => (text ? parseDrawing(text) : null))
      .catch(() => null)
    if (scenes.size > 50) scenes.delete(scenes.keys().next().value!)
    scenes.set(key, p)
  }
  return p
}

/** The drawing as an inline <svg> (natural size: one unit, one pixel). */
export function sceneSVG(scene: Scene, label: string): SVGSVGElement {
  const measure = measureText()
  const box = canvasBox(scene, measure)
  const svg = document.createElementNS(SVG_NS, 'svg')
  svg.setAttribute('class', 'dn-drawing')
  svg.setAttribute('viewBox', `${box.x} ${box.y} ${box.w} ${box.h}`)
  svg.setAttribute('width', String(box.w))
  svg.setAttribute('role', 'img')
  svg.setAttribute('aria-label', label)
  if (scene.elements.length === 0) svg.classList.add('empty')
  for (const el of scene.elements) svg.appendChild(elementNode(el, scene, measure))
  return svg
}

/**
 * Replaces the image in host with the drawing once its scene is loaded.
 * Returns false when the file has no scene (the image stays).
 */
export async function fillDrawing(host: HTMLElement, path: string, version: number | undefined, label: string): Promise<boolean> {
  const scene = await loadScene(path, version)
  if (!scene || !host.isConnected) return false
  const svg = sceneSVG(scene, label)
  const img = host.querySelector('img')
  // The written width stays; the inline drawing never grows beyond its column.
  const width = img?.getAttribute('width')
  if (width) svg.setAttribute('width', width)
  host.querySelector('svg.dn-drawing')?.remove()
  host.prepend(svg)
  host.classList.add('drawn') // hides the image
  return true
}
