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
  import { router } from '../lib/router.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import Icon from './Icon.svelte'
  import type { InsertKind } from '../lib/editor/markdownInsert'

  /** visible: false while the reading view is shown (the editor stays open). */
  let { session, visible = true }: { session: NoteSession; visible?: boolean } = $props()

  let host: HTMLDivElement | undefined = $state()
  let handle = $state.raw<EditorHandle | null>(null)
  /** Where the right-click menu opened. */
  let menu = $state<{ x: number; y: number } | null>(null)
  const m = strings.insertMenu

  function insert(kind: InsertKind) {
    menu = null
    handle?.insertMarkdown(kind)
  }

  async function clipboard(what: 'cut' | 'copy' | 'paste') {
    const view = handle?.view
    if (!view) return
    const sel = view.state.selection.main
    try {
      if (what === 'paste') {
        const text = await navigator.clipboard.readText()
        view.dispatch(view.state.replaceSelection(text), { userEvent: 'input.paste', scrollIntoView: true })
      } else {
        await navigator.clipboard.writeText(view.state.sliceDoc(sel.from, sel.to))
        if (what === 'cut') view.dispatch({ changes: { from: sel.from, to: sel.to }, userEvent: 'delete.cut' })
      }
    } catch {
      if (what === 'paste') app.toast(m.pasteFailed)
    }
    view.focus()
  }

  const blocks: [InsertKind, MenuItem['icon']][] = [
    ['bullet', 'list-bullet'],
    ['numbered', 'list-numbered'],
    ['task', 'task'],
    ['quote', 'quote'],
    ['codeblock', 'code'],
    ['table', 'table'],
    ['rule', 'rule'],
  ]
  const menuItems = $derived.by((): MenuItem[] => {
    const empty = !handle || handle.view.state.selection.main.empty
    void menu
    const items: MenuItem[] = blocks.map(([kind, icon]) => ({ label: m[kind], icon, action: () => insert(kind) }))
    if (!empty) items.push({ label: m.cut, icon: 'cut', separator: true, action: () => void clipboard('cut') })
    if (!empty) items.push({ label: m.copy, icon: 'copy', action: () => void clipboard('copy') })
    items.push({ label: m.paste, icon: 'paste', separator: empty, action: () => void clipboard('paste') })
    items.push({ label: m.cheatSheet, icon: 'book', separator: true, action: () => router.goTutorials('markdown') })
    return items
  })

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
      onContextMenu: (x, y) => (menu = { x, y }),
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

  // The title above the text; editing it renames the note (editors only).
  $effect(() => {
    const path = session.path
    handle?.setTitle({
      title: displayName(path),
      editable: !session.readOnly && app.can.organize,
      label: strings.note.titleLabel,
      rename: (title) => app.rename(path, title),
    })
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

{#snippet tools()}
  <div class="md-tools" role="group" aria-label={m.label}>
    <button type="button" class="tool" title={m.h1} aria-label={m.h1} onclick={() => insert('h1')}>H1</button>
    <button type="button" class="tool" title={m.h2} aria-label={m.h2} onclick={() => insert('h2')}>H2</button>
    <button type="button" class="tool" title={m.h3} aria-label={m.h3} onclick={() => insert('h3')}>H3</button>
    <span class="gap"></span>
    <button type="button" class="tool bold" title={m.bold} aria-label={m.bold} onclick={() => insert('bold')}>B</button>
    <button type="button" class="tool italic" title={m.italic} aria-label={m.italic} onclick={() => insert('italic')}>I</button>
    <button type="button" class="tool mono" title={m.code} aria-label={m.code} onclick={() => insert('code')}>{'</>'}</button>
    <span class="gap"></span>
    <button type="button" class="tool" title={m.link} aria-label={m.link} onclick={() => insert('link')}><Icon name="link" size={15} /></button>
    <button type="button" class="tool mono" title={m.synapse} aria-label={m.synapse} onclick={() => insert('synapse')}>[[ ]]</button>
  </div>
{/snippet}

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menuItems} onclose={() => (menu = null)} header={tools} />
{/if}

<style>
  .editor-host {
    height: 100%;
    min-height: 0;
  }
  .md-tools {
    display: flex;
    align-items: center;
    gap: 2px;
  }
  .gap {
    width: 6px;
  }
  .tool {
    display: grid;
    place-items: center;
    min-width: 28px;
    height: 28px;
    padding: 0 6px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text);
    font-size: var(--text-sm);
    font-weight: 600;
    cursor: pointer;
  }
  .tool:hover,
  .tool:focus-visible {
    background: var(--bg-hover);
    outline: none;
  }
  .italic {
    font-style: italic;
    font-weight: 500;
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
</style>
