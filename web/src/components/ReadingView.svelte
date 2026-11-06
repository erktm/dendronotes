<script lang="ts">
  import type { NoteSession } from '../lib/state/note.svelte'
  import { app } from '../lib/state/app.svelte'
  import { settings } from '../lib/state/settings.svelte'
  import { renderMarkdown, type RenderContext } from '../lib/markdown'
  import { noteURL } from '../lib/router.svelte'
  import { readNote } from '../lib/api/soma'
  import { live } from '../lib/state/live.svelte'
  import { section } from '../lib/outline'
  import { strings } from '../lib/strings'

  let { session }: { session: NoteSession } = $props()

  let article: HTMLElement | undefined = $state()

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

  function toggleTask(line: number) {
    if (session.readOnly) return
    const lines = session.content.split('\n')
    const text = lines[line]
    if (text === undefined) return
    lines[line] = text.replace(/\[( |x|X)\]/, (_m, c: string) => (c === ' ' ? '[x]' : '[ ]'))
    session.content = lines.join('\n')
    session.scheduleSave()
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
      const a = target.closest('a')
      if (!a) return
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

  $effect(() => {
    const req = app.scrollRequest
    if (req && article) {
      const target = req.slug ? article.querySelector(`[id="${CSS.escape(req.slug)}"]`) : blockAt(article, req.line)
      target?.scrollIntoView({ block: req.slug ? 'start' : 'center' })
      app.scrollRequest = null
    }
  })
</script>

<div class="note-surface" class:serif={settings.noteFont === 'serif'} class:readable={settings.readableWidth}>
  <!-- Sanitized by DOMPurify in renderMarkdown. -->
  <article class="reading" bind:this={article} use:interactions>{@html html}</article>
</div>
