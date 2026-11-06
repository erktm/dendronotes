// Cursor position and Vim mode of the open editor, for the status bar.
class EditorStatus {
  line = $state(1)
  col = $state(1)
  /** Vim mode ("normal", "insert", "visual" …) or null when Vim is off. */
  vimMode = $state<string | null>(null)
}

export const editorStatus = new EditorStatus()
