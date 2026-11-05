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
