// What is dragged over the file tree, kept in one place: at most one folder
// or row shows where a drop would land, and it clears after every drop and
// when the drag ends, wherever that happens.

export type Place = 'before' | 'after'

class TreeDrag {
  /** Entries of the tree dragged in this tab (drag-over events cannot read the drag data). */
  paths: string[] = []
  /** The folder a drop would go into ('' the top level), or null. */
  into = $state<string | null>(null)
  /** The row a dragged entry would be placed before or after (sorting by hand). */
  place = $state<{ path: string; at: Place } | null>(null)

  /** Shows where a drop would land (drag-over fires often: only changes count). */
  show(into: string | null, place: { path: string; at: Place } | null): void {
    if (this.into !== into) this.into = into
    if (place?.path !== this.place?.path || place?.at !== this.place?.at) this.place = place
  }

  /** Nothing is shown any more (a drop happened, or the drag left the tree). */
  clear(): void {
    this.show(null, null)
  }

  /** The drag is over. */
  reset(): void {
    this.paths = []
    this.clear()
  }
}

export const treeDrag = new TreeDrag()
