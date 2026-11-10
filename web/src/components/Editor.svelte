<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { collab } from '@codemirror/collab'
  import { createEditor, type EditorHandle } from '../lib/editor/setup'
  import { readNote } from '../lib/api/soma'
  import { outline } from '../lib/outline'
  import type { NoteSession } from '../lib/state/note.svelte'
  import { app } from '../lib/state/app.svelte'
  import { settings } from '../lib/state/settings.svelte'
  import { editorStatus } from '../lib/state/status.svelte'
  import { dirname, displayName } from '../lib/paths'
  import { fileKind, pastedName } from '../lib/files'
  import { comments } from '../lib/state/comments.svelte'
  import { findAnchor, makeAnchor } from '../lib/anchors'
  import { isDrawingPath } from '../lib/drawing/paths'
  import { strings } from '../lib/strings'

  /** visible: false while the reading view is shown (the editor stays open). */
  let { session, visible = true }: { session: NoteSession; visible?: boolean } = $props()

  let host: HTMLDivElement | undefined = $state()
  let handle = $state.raw<EditorHandle | null>(null)

  onMount(() => {
    if (!host) return
    const s = session
    const editor = createEditor(host, {
      doc: s.content,
      collab: collab({ startVersion: s.startVersion, clientID: s.clientID }),
      livePreview: settings.livePreview,
      readOnly: s.readOnly,
      label: strings.note.contentLabel(displayName(s.path)),
      placeholder: strings.note.placeholder,
      host: {
        resolve: (target, markdown) => app.resolveSynapse(target, markdown, s.path),
        open: (target, markdown) => void app.openSynapse(target, markdown, s.path),
        resolveFile: (target, markdown) => app.resolveFile(target, markdown, s.path),
        openFile: (path) => void app.navigate(path),
        fileVersion: (path) => app.entries.find((e) => e.path === path)?.mtime,
      },
      notes: () => app.notePaths,
      embeddable: () => app.filePaths.filter((p) => isDrawingPath(p) || fileKind(p) === 'image'),
      headings: async (path) => outline((await readNote(path)).content),
      onChange: (view) => s.changed(view),
      onSelection: (view) => s.selectionChanged(view),
      onCursor: (line, col) => {
        editorStatus.line = line
        editorStatus.col = col
      },
      onVimMode: (mode) => (editorStatus.vimMode = mode),
      onFiles: (files, pos) => void embedFiles(files, pos),
      comments: {
        label: strings.comments.add,
        comment: (from, to) => {
          comments.draft = makeAnchor(editor.view.state.doc.toString(), from, to)
          comments.draftFocus++
          app.commentsOpen = true
        },
        focus: (id) => {
          comments.active = id
          app.commentsOpen = true
        },
      },
    })
    handle = editor
    editor.restore(s.lastSelection, s.lastScroll)
    s.attach(editor.view)
    // A new note, or the editor started over while someone was typing.
    if ((s.autofocus || s.lastFocused) && visible) {
      editor.focus()
      s.autofocus = false
      s.lastFocused = false
    }
    return () => {
      s.detach(editor.view)
      editor.destroy()
      editorStatus.vimMode = null
    }
  })

  // The others' cursors.
  $effect(() => {
    const peers = session.peers
    handle?.setPeers(peers.filter((p) => p.sel).map((p) => ({ id: p.id, name: p.name, color: p.color, anchor: p.sel!.anchor, head: p.sel!.head })))
  })

  // The passages comments are about: found anew when the comments or the
  // document change from outside; while typing, the marks move along.
  $effect(() => {
    const threads = comments.threads
    const active = comments.active
    void session.generation
    if (!handle || comments.path !== session.path) return
    const text = handle.view.state.doc.toString()
    const ranges = []
    for (const t of threads) {
      const span = findAnchor(text, t.root)
      if (span) ranges.push({ id: t.root.id, ...span, active: t.root.id === active, resolved: !!t.root.resolvedAt })
    }
    handle.setComments(ranges)
  })

  // A thread chosen in the panel: show its passage.
  $effect(() => {
    if (!comments.reveal || !handle || !visible) return
    const h = handle
    untrack(() => {
      const pos = comments.active !== null ? h.commentPositions().get(comments.active) : undefined
      if (pos) h.reveal(pos.from, pos.to)
    })
  })

  // Shown again after the reading view: measure, and take the focus.
  $effect(() => {
    if (visible && handle) {
      handle.remeasure()
      if (session.autofocus) {
        handle.focus()
        session.autofocus = false
      }
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

  // Files added, renamed or removed: re-resolve synapses and attachments;
  // a drawing saved anew is drawn again.
  const drawingStamp = $derived(
    app.entries
      .filter((e) => isDrawingPath(e.path))
      .map((e) => e.mtime)
      .join(),
  )
  $effect(() => {
    void app.filePaths
    void drawingStamp
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
    if (req && handle && visible) {
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
