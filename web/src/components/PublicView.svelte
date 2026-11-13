<script lang="ts">
  // The page of a public link (/p/<token>): one note or drawing, read-only,
  // without signing in. Files load through the link (setPublicToken), and
  // only those the note refers to; synapses lead nowhere here and show as text.
  import { onMount, tick } from 'svelte'
  import Wordmark from './Wordmark.svelte'
  import { ApiError } from '../lib/api/client'
  import { setPublicToken } from '../lib/api/base'
  import { readShared, type Shared } from '../lib/api/links'
  import { highlightCodeBlocks } from '../lib/codeHighlight'
  import { renderMarkdown, type RenderContext } from '../lib/markdown'
  import { displayName } from '../lib/paths'
  import { drawingTitle } from '../lib/drawing/paths'
  import { resolveFile } from '../lib/synapses'
  import { strings } from '../lib/strings'

  let { token }: { token: string } = $props()
  const t = strings.publicPage

  let shared = $state.raw<Shared | null>(null)
  let problem = $state<string | null>(null)
  let article: HTMLElement | undefined = $state()
  let drawingHost: HTMLElement | undefined = $state()

  const name = $derived(!shared ? '' : shared.kind === 'drawing' ? drawingTitle(shared.path) : displayName(shared.path))
  const html = $derived(shared?.kind === 'note' ? renderMarkdown(shared.content ?? '', contextFor(shared)) : '')

  function contextFor(s: Shared): RenderContext {
    return {
      resolve: () => null,
      resolveFile: (target, markdown) => resolveFile({ target, markdown }, s.path, s.files ?? []),
      noteURL: () => '#',
    }
  }

  onMount(async () => {
    setPublicToken(token)
    try {
      shared = await readShared(token)
      document.title = shared.kind === 'drawing' ? drawingTitle(shared.path) : displayName(shared.path)
    } catch (err) {
      problem = err instanceof ApiError && err.code === 'link_expired' ? t.expired : t.notFound
    }
  })

  // After the note is rendered: synapses as text, code colors, drawings.
  $effect(() => {
    if (!article || !html) return
    void tick().then(() => {
      if (!article) return
      for (const a of article.querySelectorAll('a.synapse')) a.replaceWith(document.createTextNode(a.textContent ?? ''))
      void highlightCodeBlocks(article)
      for (const host of article.querySelectorAll<HTMLElement>('.drawing-embed[data-drawing]')) {
        const path = host.dataset.drawing ?? ''
        const label = host.querySelector('img')?.alt || path
        void import('../lib/drawing/embed').then((m) => m.fillDrawing(host, path, undefined, label))
      }
    })
  })

  // A shared drawing is drawn from its scene, like everywhere in the app.
  $effect(() => {
    const host = drawingHost
    const svg = shared?.kind === 'drawing' ? shared.svg : null
    if (!host || !svg) return
    void Promise.all([import('../lib/drawing/svg'), import('../lib/drawing/embed')]).then(([svgMod, embed]) => {
      const scene = svgMod.parseDrawing(svg)
      if (scene) host.replaceChildren(embed.sceneSVG(scene, t.drawingLabel(name)))
      else problem = t.notFound
    })
  })
</script>

<div class="public">
  <header>
    <span class="badge">{t.readOnly}</span>
    <span class="by">{t.sharedWith} <Wordmark /></span>
  </header>
  <main class="note-surface readable">
    {#if problem}
      <p class="problem">{problem}</p>
    {:else if !shared}
      <p class="loading">{t.loading}</p>
    {:else if shared.kind === 'note'}
      <div class="note-title in-reading"><h1 class="note-title-text">{name}</h1></div>
      <!-- Sanitized by DOMPurify in renderMarkdown. -->
      <article class="reading" bind:this={article}>{@html html}</article>
    {:else}
      <div class="drawing-page">
        <h1 class="note-title-text">{name}</h1>
        <div class="drawing" bind:this={drawingHost}></div>
      </div>
    {/if}
  </main>
</div>

<style>
  .public {
    min-height: 100%;
    background: var(--bg);
    color: var(--text);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 20px;
    border-bottom: 1px solid var(--line);
    background: var(--bg-raised);
    font-size: var(--text-sm);
  }
  .badge {
    padding: 2px 10px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--text);
    font-weight: 550;
  }
  .by {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--text-muted);
  }
  main {
    padding-bottom: var(--space-6);
  }
  .problem,
  .loading {
    max-width: 640px;
    margin: 80px auto;
    padding: 0 20px;
    color: var(--text-muted);
    text-align: center;
  }
  .drawing-page {
    max-width: 1200px;
    margin: 0 auto;
    padding: 40px 48px;
  }
  .drawing {
    margin-top: 24px;
  }
  .drawing :global(svg) {
    max-width: 100%;
    height: auto;
  }
</style>
