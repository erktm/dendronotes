// Svelte actions. Styles are set through the CSSOM (element.style), which
// the strict Content-Security-Policy allows – inline style attributes are not.

/** Sets CSS custom properties (or other style properties) on an element. */
export function cssVars(node: HTMLElement, vars: Record<string, string | number>) {
  const apply = (v: Record<string, string | number>) => {
    for (const [key, value] of Object.entries(v)) node.style.setProperty(key, String(value))
  }
  apply(vars)
  return { update: apply }
}

/** Focuses (and optionally selects) an element when it is mounted. */
export function focusOnMount(node: HTMLElement, select = false) {
  queueMicrotask(() => {
    node.focus()
    if (select && node instanceof HTMLInputElement) node.select()
  })
}

/**
 * Closes a modal <dialog> on a click outside its box (on the backdrop), like
 * Escape does. The click must also start outside, so selecting text inside
 * and letting go outside keeps the dialog open.
 */
export function closeOnBackdrop(node: HTMLDialogElement) {
  const outside = (e: MouseEvent) => {
    if (e.target !== node) return false
    const r = node.getBoundingClientRect()
    return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom
  }
  let startedOutside = false
  const onpointerdown = (e: PointerEvent) => {
    startedOutside = outside(e)
  }
  const onclick = (e: MouseEvent) => {
    if (startedOutside && outside(e)) node.close()
    startedOutside = false
  }
  node.addEventListener('pointerdown', onpointerdown)
  node.addEventListener('click', onclick)
  return {
    destroy() {
      node.removeEventListener('pointerdown', onpointerdown)
      node.removeEventListener('click', onclick)
    },
  }
}
