<script lang="ts">
  import { displayName } from '../lib/paths'
  import { untrack } from 'svelte'
  import type { NoteSession } from '../lib/state/note.svelte'
  import { app } from '../lib/state/app.svelte'
  import { settings } from '../lib/state/settings.svelte'
  import { renderMarkdown, type RenderContext } from '../lib/markdown'
  import { noteURL } from '../lib/router.svelte'
  import { readNote } from '../lib/api/soma'
  import { live } from '../lib/state/live.svelte'
  import { section } from '../lib/outline'
  import { strings } from '../lib/strings'
  import { comments } from '../lib/state/comments.svelte'
  import { anchorFromRendered, CONTEXT, findAnchor, type Anchor } from '../lib/anchors'
  import { cssVars } from '../lib/actions'
  import { iconPath } from './Icon.svelte'
  import { isDrawingPath } from '../lib/drawing/paths'

  let { session }: { session: NoteSession } = $props()

  let article: HTMLElement | undefined = $state()
  let surface: HTMLDivElement | undefined = $state()

  /** Resolves synapses and attachments as written in the note at path. */
  const contextFor = (path: string): RenderContext => ({
    resolve: (target, markdown) => app.resolveSynapse(target, markdown, path),
    resolveFile: (target, markdown) => app.resolveFile(target, markdown, path),
    noteURL: (p) => noteURL(app.somaId, p),
  })

  const html = $derived(renderMarkdown(session.content, contextFor(session.path)))

  // --- Embedded notes (![[Note]], ![[Note#Heading]]) ---------------------

  const MAX_EMBED_DEPTH = 3
  let generation = 0

  /** Loads and renders the embedded notes below root; nested ones follow recursively. */
  async function fillEmbeds(root: ParentNode, chain: readonly string[], gen: number): Promise<void> {
    // Only this level's embeds; nested ones are filled after their parent.
    const owner = root instanceof Element ? root.closest('.embed-note') : null
    const pending = [...root.querySelectorAll<HTMLElement>('.embed-note[data-embed]:not([data-filled])')].filter(
      (el) => el.parentElement?.closest('.embed-note') === owner,
    )
    await Promise.all(
      pending.map(async (el) => {
        el.dataset.filled = ''
        const path = el.dataset.embed ?? ''
        const body = el.querySelector<HTMLElement>(':scope > .embed-body')
        if (!body) return
        if (chain.includes(path) || chain.length > MAX_EMBED_DEPTH) {
          body.textContent = strings.reading.embedTooDeep
          return
        }
        let content: string
        try {
          content = (await readNote(path)).content
        } catch {
          if (gen === generation) body.textContent = strings.reading.embedMissing(path)
          return
        }
        if (gen !== generation) return // the note was rendered again meanwhile
        const heading = el.dataset.embedHeading
        // Sanitized by DOMPurify in renderMarkdown.
        body.innerHTML = renderMarkdown(heading ? section(content, heading) : content, contextFor(path))
        // Checkboxes belong to the other note: shown, not toggled from here.
        for (const box of body.querySelectorAll<HTMLInputElement>('input.task-box')) box.disabled = true
        fillDrawings(body)
        await fillEmbeds(body, [...chain, path], gen)
      }),
    )
  }

  $effect(() => {
    void html
    void live.version // embedded notes may have changed
    if (!article) return
    const gen = ++generation
    for (const el of article.querySelectorAll<HTMLElement>('.embed-note[data-filled]')) delete el.dataset.filled
    void fillEmbeds(article, [session.path], gen)
  })

  // --- Drawings (![[Plan.drawing.svg]]) -----------------------------------
  // Drawn in place from their scene; a click edits them (writers) or shows
  // them large (everyone else, and the button in the corner).

  let viewing = $state<string | null>(null)

  /** Changes when a drawing file changes, so embeds load it again. */
  const drawingStamp = $derived(
    app.entries
      .filter((e) => isDrawingPath(e.path))
      .map((e) => `${e.path}:${e.mtime}`)
      .join('|'),
  )

  function fillDrawings(root: ParentNode) {
    for (const host of root.querySelectorAll<HTMLElement>('.drawing-embed[data-drawing]')) {
      const path = host.dataset.drawing ?? ''
      const version = app.entries.find((e) => e.path === path)?.mtime
      if (host.dataset.filled === String(version)) continue
      host.dataset.filled = String(version)
      if (!host.querySelector('.drawing-zoom')) host.append(zoomButton())
      const label = host.querySelector('img')?.alt || path
      // The drawing code loads with the first drawing shown.
      void import('../lib/drawing/embed').then((m) => m.fillDrawing(host, path, version, label))
    }
  }

  function zoomButton(): HTMLButtonElement {
    const b = document.createElement('button')
    b.type = 'button'
    b.className = 'drawing-zoom'
    b.title = strings.drawing.fullscreen
    b.setAttribute('aria-label', strings.drawing.fullscreen)
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
    svg.setAttribute('viewBox', '0 0 24 24')
    svg.setAttribute('width', '16')
    svg.setAttribute('height', '16')
    svg.setAttribute('aria-hidden', 'true')
    const p = document.createElementNS('http://www.w3.org/2000/svg', 'path')
    p.setAttribute('d', iconPath('expand'))
    svg.append(p)
    b.append(svg)
    return b
  }

  $effect(() => {
    void html
    void drawingStamp
    if (article) {
      const root = article
      untrack(() => fillDrawings(root))
    }
  })

  function toggleTask(line: number) {
    session.toggleTask(line)
  }

  /** Synapses navigate inside the app; checkboxes edit the note. */
  function interactions(node: HTMLElement) {
    const onclick = (e: MouseEvent) => {
      const target = e.target as HTMLElement
      // Inside an embedded note, synapses are relative to that note.
      const embed = target.closest<HTMLElement>('.embed-note .embed-body')?.closest<HTMLElement>('.embed-note')
      const from = embed?.dataset.embed ?? session.path
      if (target instanceof HTMLInputElement && target.classList.contains('task-box')) {
        e.preventDefault()
        if (!embed) toggleTask(Number(target.dataset.line))
        return
      }
      const drawing = target.closest<HTMLElement>('.drawing-embed[data-drawing]')
      if (drawing) {
        e.preventDefault()
        const path = drawing.dataset.drawing ?? ''
        if (target.closest('.drawing-zoom') || !app.can.write) viewing = path
        else void app.navigate(path)
        return
      }
      const a = target.closest('a')
      if (!a) {
        const id = threadAt(e.clientX, e.clientY)
        if (id !== null && window.getSelection()?.isCollapsed !== false) {
          comments.active = id
          app.commentsOpen = true
        }
        return
      }
      if (a.dataset.synapse !== undefined) {
        e.preventDefault()
        void app.openSynapse(a.dataset.synapse, a.dataset.markdown === 'true', from)
      } else if (a.getAttribute('href')?.startsWith('#')) {
        e.preventDefault()
        node.querySelector(`[id="${CSS.escape(a.getAttribute('href')!.slice(1))}"]`)?.scrollIntoView({ block: 'start' })
      }
    }
    node.addEventListener('click', onclick)
    return { destroy: () => node.removeEventListener('click', onclick) }
  }

  /** The last top-level block starting at or before a source line. */
  function blockAt(root: HTMLElement, line: number): Element | null {
    let found: Element | null = null
    for (const el of root.querySelectorAll('[data-source-line]')) {
      if (Number((el as HTMLElement).dataset.sourceLine) > line) break
      found = el
    }
    return found
  }

  // --- Comments --------------------------------------------------------------
  // Passages are marked with the CSS Custom Highlight API: the rendered (and
  // sanitized) HTML stays untouched. Browsers without it show no marks.

  /** The rendered text and where each text node starts in it (drawings left out). */
  function textIndex(root: HTMLElement): { text: string; nodes: { node: Text; start: number }[] } {
    const nodes: { node: Text; start: number }[] = []
    let text = ''
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: (n) => (n.parentElement?.closest('.drawing-embed') ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT),
    })
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      nodes.push({ node: n as Text, start: text.length })
      text += n.nodeValue ?? ''
    }
    return { text, nodes }
  }

  function rangeOf(index: ReturnType<typeof textIndex>, from: number, to: number): Range | null {
    const at = (pos: number) => {
      let lo = 0
      let hi = index.nodes.length - 1
      while (lo < hi) {
        const mid = (lo + hi + 1) >> 1
        if ((index.nodes[mid]?.start ?? 0) <= pos) lo = mid
        else hi = mid - 1
      }
      const n = index.nodes[lo]
      return n ? { node: n.node, offset: Math.min(pos - n.start, n.node.length) } : null
    }
    const a = at(from)
    const b = at(to)
    if (!a || !b) return null
    const r = document.createRange()
    r.setStart(a.node, a.offset)
    r.setEnd(b.node, b.offset)
    return r
  }

  let marked: { id: number; range: Range }[] = []
  const highlights = typeof CSS !== 'undefined' && 'highlights' in CSS ? CSS.highlights : null

  $effect(() => {
    void html
    const threads = comments.threads
    const active = comments.active
    if (!article || !highlights || comments.path !== session.path) return
    const index = textIndex(article)
    marked = []
    const normal: Range[] = []
    const focused: Range[] = []
    for (const th of threads) {
      if (th.root.resolvedAt || !th.root.quote) continue
      const span = findAnchor(index.text, th.root)
      const range = span && rangeOf(index, span.from, span.to)
      if (!range) continue
      marked.push({ id: th.root.id, range })
      ;(th.root.id === active ? focused : normal).push(range)
    }
    highlights.set('dn-comment', new Highlight(...normal))
    highlights.set('dn-comment-active', new Highlight(...focused))
    return () => {
      highlights.delete('dn-comment')
      highlights.delete('dn-comment-active')
    }
  })

  /** The thread whose passage is at a point (for clicks). */
  function threadAt(x: number, y: number): number | null {
    const doc = document as Document & { caretPositionFromPoint?: (x: number, y: number) => { offsetNode: Node; offset: number } | null }
    let node: Node | null = null
    let offset = 0
    if (doc.caretPositionFromPoint) {
      const p = doc.caretPositionFromPoint(x, y)
      if (p) ({ offsetNode: node, offset } = p)
    } else {
      const r = document.caretRangeFromPoint?.(x, y)
      if (r) ({ startContainer: node, startOffset: offset } = r)
    }
    if (!node) return null
    for (const m of marked) {
      try {
        if (m.range.isPointInRange(node, offset)) return m.id
      } catch {
        // a point in another document part
      }
    }
    return null
  }

  // A thread chosen in the panel: scroll its passage into view.
  $effect(() => {
    if (!comments.reveal) return
    const id = untrack(() => comments.active)
    const m = marked.find((x) => x.id === id)
    const el = m?.range.startContainer.parentElement
    el?.scrollIntoView({ block: 'center', behavior: 'smooth' })
  })

  // A selection gets a small button to comment on it.
  let selection = $state.raw<{ x: number; y: number; anchor: Anchor } | null>(null)

  function selected() {
    const sel = window.getSelection()
    const range = sel && sel.rangeCount && !sel.isCollapsed ? sel.getRangeAt(0) : null
    if (!range || !article || !surface || !article.contains(range.commonAncestorContainer) || !sel?.toString().trim()) {
      selection = null
      return
    }
    const index = textIndex(article)
    // Where the selection starts in the indexed text (drawings left out).
    const node = index.nodes.find((n) => n.node === range.startContainer)
    let start: number
    if (node) {
      start = node.start + range.startOffset
    } else {
      const before = document.createRange()
      before.setStart(article, 0)
      before.setEnd(range.startContainer, range.startOffset)
      start = before.toString().length
    }
    const text = index.text
    const quote = range.toString()
    const anchor = anchorFromRendered(
      session.content,
      quote,
      text.slice(Math.max(0, start - CONTEXT), start),
      text.slice(start + quote.length, start + quote.length + CONTEXT),
    )
    const box = surface.getBoundingClientRect()
    const rect = range.getBoundingClientRect()
    selection = { x: Math.max(0, rect.right - box.left - 40), y: rect.bottom - box.top + 6, anchor }
  }

  function comment() {
    if (!selection) return
    comments.draft = selection.anchor
    comments.draftFocus++
    app.commentsOpen = true
    selection = null
    window.getSelection()?.removeAllRanges()
  }

  $effect(() => {
    const update = () => setTimeout(selected, 0)
    document.addEventListener('selectionchange', update)
    return () => document.removeEventListener('selectionchange', update)
  })

  $effect(() => {
    const req = app.scrollRequest
    if (req && article) {
      const target = req.slug ? article.querySelector(`[id="${CSS.escape(req.slug)}"]`) : blockAt(article, req.line)
      target?.scrollIntoView({ block: req.slug ? 'start' : 'center' })
      app.scrollRequest = null
    }
  })
</script>

<div class="note-surface" class:serif={settings.noteFont === 'serif'} class:readable={settings.readableWidth} bind:this={surface}>
  <!-- The title, as in the editor: the file name, not part of the text. -->
  <div class="note-title in-reading"><h1 class="note-title-text">{displayName(session.path)}</h1></div>
  <!-- Sanitized by DOMPurify in renderMarkdown. -->
  <article class="reading" bind:this={article} use:interactions>{@html html}</article>
  {#if viewing}
    {#await import('./DrawingLightbox.svelte') then { default: DrawingLightbox }}
      <DrawingLightbox path={viewing} onclose={() => (viewing = null)} />
    {/await}
  {/if}
  {#if selection}
    <button
      type="button"
      class="comment-on"
      use:cssVars={{ '--x': `${selection.x}px`, '--y': `${selection.y}px` }}
      onmousedown={(e) => e.preventDefault()}
      onclick={comment}
    >
      {strings.comments.add}
    </button>
  {/if}
</div>

<style>
  .note-surface {
    position: relative;
  }
  .comment-on {
    position: absolute;
    z-index: 5;
    top: var(--y);
    left: var(--x);
    padding: 3px 10px;
    border: 1px solid var(--line-strong);
    border-radius: 999px;
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow);
    font-size: var(--text-xs);
    font-weight: 600;
    cursor: pointer;
  }
  .comment-on:hover {
    background: var(--bg-hover);
  }
  :global(::highlight(dn-comment)) {
    background-color: color-mix(in srgb, var(--warn) 18%, transparent);
    text-decoration: underline 2px color-mix(in srgb, var(--warn) 60%, transparent);
    text-underline-offset: 3px;
  }
  :global(::highlight(dn-comment-active)) {
    background-color: color-mix(in srgb, var(--warn) 34%, transparent);
    text-decoration: underline 2px var(--warn);
    text-underline-offset: 3px;
  }
</style>
