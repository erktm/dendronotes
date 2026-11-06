<script lang="ts">
  import type { NoteSession } from '../lib/state/note.svelte'
  import { app } from '../lib/state/app.svelte'
  import { settings } from '../lib/state/settings.svelte'
  import { renderMarkdown } from '../lib/markdown'

  let { session }: { session: NoteSession } = $props()

  let article: HTMLElement | undefined = $state()

  const html = $derived(
    renderMarkdown(session.content, {
      resolve: (target, markdown) => app.resolveSynapse(target, markdown, session.path),
    }),
  )

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
      if (target instanceof HTMLInputElement && target.classList.contains('task-box')) {
        e.preventDefault()
        toggleTask(Number(target.dataset.line))
        return
      }
      const a = target.closest('a')
      if (!a) return
      if (a.dataset.synapse !== undefined) {
        e.preventDefault()
        void app.openSynapse(a.dataset.synapse, a.dataset.markdown === 'true', session.path)
      } else if (a.getAttribute('href')?.startsWith('#')) {
        e.preventDefault()
        node.querySelector(`[id="${CSS.escape(a.getAttribute('href')!.slice(1))}"]`)?.scrollIntoView({ block: 'start' })
      }
    }
    node.addEventListener('click', onclick)
    return { destroy: () => node.removeEventListener('click', onclick) }
  }

  $effect(() => {
    const req = app.scrollRequest
    if (req && article) {
      article.querySelector(`[id="${CSS.escape(req.slug)}"]`)?.scrollIntoView({ block: 'start' })
      app.scrollRequest = null
    }
  })
</script>

<div class="note-surface" class:serif={settings.noteFont === 'serif'} class:readable={settings.readableWidth}>
  <!-- Sanitized by DOMPurify in renderMarkdown. -->
  <article class="reading" bind:this={article} use:interactions>{@html html}</article>
</div>
