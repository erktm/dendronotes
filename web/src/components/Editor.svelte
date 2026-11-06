<script lang="ts">
  import { onMount } from 'svelte'
  import { createEditor, type EditorHandle } from '../lib/editor/setup'
  import { readNote } from '../lib/api/soma'
  import { outline } from '../lib/outline'
  import type { NoteSession } from '../lib/state/note.svelte'
  import { app } from '../lib/state/app.svelte'
  import { settings } from '../lib/state/settings.svelte'
  import { editorStatus } from '../lib/state/status.svelte'
  import { dirname, displayName } from '../lib/paths'
  import { pastedName } from '../lib/files'
  import { strings } from '../lib/strings'

  let { session }: { session: NoteSession } = $props()

  let host: HTMLDivElement | undefined = $state()
  let handle = $state.raw<EditorHandle | null>(null)

  onMount(() => {
    if (!host) return
    const s = session
    const editor = createEditor(host, {
      doc: s.content,
      livePreview: settings.livePreview,
      readOnly: s.readOnly,
      label: strings.note.contentLabel(displayName(s.path)),
      placeholder: strings.note.placeholder,
      host: {
        resolve: (target, markdown) => app.resolveSynapse(target, markdown, s.path),
        open: (target, markdown) => void app.openSynapse(target, markdown, s.path),
        resolveFile: (target, markdown) => app.resolveFile(target, markdown, s.path),
      },
      notes: () => app.notePaths,
      headings: async (path) => outline((await readNote(path)).content),
      onChange: (doc) => {
        s.content = doc
        s.scheduleSave()
      },
      onCursor: (line, col) => {
        editorStatus.line = line
        editorStatus.col = col
      },
      onVimMode: (mode) => (editorStatus.vimMode = mode),
      onFiles: (files, pos) => void embedFiles(files, pos),
    })
    handle = editor
    if (s.autofocus) {
      editor.focus()
      s.autofocus = false
    }
    return () => {
      editor.destroy()
      editorStatus.vimMode = null
    }
  })

  $effect(() => {
    handle?.setLivePreview(settings.livePreview)
  })

  $effect(() => {
    void handle?.setVim(settings.vim)
  })

  $effect(() => {
    handle?.setReadOnly(session.readOnly)
  })

  // Content replaced from outside (reload from disk).
  $effect(() => {
    const content = session.content
    if (handle && content !== handle.doc()) handle.replaceDoc(content)
  })

  // Files added, renamed or removed: re-resolve synapses and attachments.
  $effect(() => {
    void app.filePaths
    handle?.refresh()
  })

  /** Clipboard images carry generic names; give them a dated one. */
  const generic = (f: File) => f.name === '' || /^image\.(png|jpe?g|gif|webp)$/i.test(f.name)

  /** Uploads pasted or dropped files next to the note and embeds them. */
  async function embedFiles(files: File[], pos: number) {
    const s = session
    if (s.readOnly) return
    if (files.some((f) => f.size > 2 << 20)) app.toast(strings.files.uploading(files.length), 'info')
    const stored = await app.upload(files, dirname(s.path), (f) => (generic(f) ? pastedName(f.type) : f.name))
    if (stored.length === 0 || !handle || app.note !== s) return
    handle.insertAt(pos, stored.map((p) => `![[${app.embedTarget(p, s.path)}]]`).join('\n'))
  }

  $effect(() => {
    const req = app.scrollRequest
    if (req && handle) {
      handle.scrollToLine(req.line)
      app.scrollRequest = null
    }
  })
</script>

<div
  class="editor-host note-surface"
  class:serif={settings.noteFont === 'serif'}
  class:readable={settings.readableWidth}
  bind:this={host}
></div>

<style>
  .editor-host {
    height: 100%;
    min-height: 0;
  }
</style>
