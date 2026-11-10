// Names of drawings ("Name.drawing.svg"), without the rest of lib/drawing,
// so the app can tell drawings apart without loading their code.

export const SUFFIX = '.drawing.svg'

/** Whether a path names a drawing. */
export function isDrawingPath(path: string): boolean {
  return path.toLowerCase().endsWith(SUFFIX)
}

/** "Plan.drawing.svg" for "Plan" (and for "Plan.drawing"). */
export function withDrawingExtension(name: string): string {
  if (isDrawingPath(name)) return name
  return name.replace(/\.drawing$/i, '') + SUFFIX
}

/** "Plan" for "Ideas/Plan.drawing.svg". */
export function drawingTitle(path: string): string {
  const name = path.slice(path.lastIndexOf('/') + 1)
  return isDrawingPath(name) ? name.slice(0, -SUFFIX.length) : name
}
