// Edit or read: the stored choice, except that notes you can't change
// (viewers, symbolic links) always open in the reading view.

import { settings, type ViewMode } from './state/settings.svelte'

export function viewOf(note: { readOnly: boolean }): ViewMode {
  return note.readOnly ? 'read' : settings.view
}
