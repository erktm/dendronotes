<script lang="ts">
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { live } from '../lib/state/live.svelte'
  import * as api from '../lib/api/soma'
  import type { Hit, TagCount } from '../lib/api/soma'
  import { snippetParts } from '../lib/snippet'
  import { ApiError } from '../lib/api/client'
  import { errorMessage } from '../lib/errors'
  import { dirname, displayName } from '../lib/paths'
  import { strings } from '../lib/strings'

  let input: HTMLInputElement | undefined = $state()
  let hits = $state.raw<Hit[]>([])
  let complete = $state(true)
  let searched = $state('')
  let active = $state(0)
  let tags = $state.raw<TagCount[] | null>(null)

  const query = $derived(app.searchQuery.trim())

  // Focus the field whenever the search is opened.
  $effect(() => {
    void app.searchFocus
    input?.focus()
    input?.select()
  })

  // Search as you type; again whenever the soma changes.
  $effect(() => {
    const q = query
    void live.version
    if (!q) {
      hits = []
      searched = ''
      return
    }
    const ctrl = new AbortController()
    const timer = setTimeout(async () => {
      try {
        const res = await api.search(q, ctrl.signal)
        if (q !== searched) active = 0
        hits = res.hits
        complete = res.complete
        searched = q
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') return
        if (!(err instanceof ApiError && err.status === 401)) app.toast(errorMessage(err))
      }
    }, 120)
    return () => {
      clearTimeout(timer)
      ctrl.abort()
    }
  })

  // Tags for the empty state.
  $effect(() => {
    if (query) return
    void live.version
    const ctrl = new AbortController()
    api.tags(ctrl.signal).then(
      (t) => (tags = t),
      () => {},
    )
    return () => ctrl.abort()
  })

  function open(hit: Hit) {
    void app.openAt(hit.path, hit.line)
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' && hits.length) {
      e.preventDefault()
      active = (active + 1) % hits.length
      document.getElementById(`hit-${active}`)?.scrollIntoView({ block: 'nearest' })
    } else if (e.key === 'ArrowUp' && hits.length) {
      e.preventDefault()
      active = (active - 1 + hits.length) % hits.length
      document.getElementById(`hit-${active}`)?.scrollIntoView({ block: 'nearest' })
    } else if (e.key === 'Enter') {
      const hit = hits[active]
      if (!hit) return
      e.preventDefault()
      open(hit)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      if (app.searchQuery) app.searchQuery = ''
      else app.sidebarView = 'files'
    }
  }
</script>

<div class="search" role="search">
  <header class="head">
    <label class="field">
      <Icon name="search" size={16} />
      <span class="visually-hidden">{strings.search.label}</span>
      <input
        bind:this={input}
        bind:value={app.searchQuery}
        type="search"
        placeholder={strings.search.placeholder}
        autocomplete="off"
        spellcheck="false"
        role="combobox"
        aria-expanded={hits.length > 0}
        aria-controls="search-results"
        aria-activedescendant={hits.length ? `hit-${active}` : undefined}
        {onkeydown}
      />
    </label>
    {#if app.searchQuery}
      <button type="button" class="clear" aria-label={strings.search.clear} title={strings.search.clear} onclick={() => ((app.searchQuery = ''), input?.focus())}>
        <Icon name="close" size={16} />
      </button>
    {/if}
  </header>

  <div class="body">
    {#if !query}
      <p class="hint">{strings.search.hint}</p>
      <h2>{strings.search.tags}</h2>
      {#if tags && tags.length === 0}
        <p class="hint">{strings.search.noTags}</p>
      {:else if tags}
        <ul class="tags">
          {#each tags as t (t.tag)}
            <li>
              <button type="button" class="tag" onclick={() => app.openSearch(`#${t.tag}`)}>
                #{t.tag}<span class="n">{t.count}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    {:else}
      <p class="status" role="status">
        {#if searched === query}
          {hits.length === 0 ? strings.search.empty : strings.search.count(hits.length)}
        {/if}
      </p>
      {#if !complete}
        <p class="hint">{strings.search.incomplete}</p>
      {/if}
      <div id="search-results" class="hits" role="listbox" tabindex="-1" aria-label={strings.search.results}>
        {#each hits as hit, i (hit.path)}
            <button
              type="button"
              role="option"
              id="hit-{i}"
              class:active={i === active}
              aria-selected={i === active}
              tabindex="-1"
              onclick={() => open(hit)}
              onmousemove={() => (active = i)}
            >
              <span class="title">
                <span class="name">{displayName(hit.path)}</span>
                {#if dirname(hit.path)}<span class="folder">{dirname(hit.path)}</span>{/if}
              </span>
              {#if hit.snippet}
                <span class="snippet">
                  {#each snippetParts(hit.snippet) as p, j (j)}{#if p.mark}<mark>{p.text}</mark>{:else}{p.text}{/if}{/each}
                </span>
              {:else}
                <span class="snippet faint">{strings.search.nameMatch}</span>
              {/if}
              {#if hit.tags?.length}
                <span class="hit-tags">
                  {#each hit.tags as t (t)}<span>#{t}</span>{/each}
                </span>
              {/if}
            </button>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .search {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-height: 0;
  }
  .head {
    display: flex;
    flex: none;
    align-items: center;
    gap: 4px;
    height: 48px;
    padding: 0 var(--space-2) 0 var(--space-3);
    border-bottom: 1px solid var(--line);
  }
  .field {
    display: flex;
    flex: 1;
    align-items: center;
    gap: 8px;
    min-width: 0;
    height: 32px;
    padding: 0 10px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg);
    color: var(--text-muted);
  }
  .field:focus-within {
    border-color: var(--accent);
    outline: 2px solid color-mix(in srgb, var(--accent) 25%, transparent);
  }
  input {
    flex: 1;
    min-width: 0;
    border: 0;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
    outline: none;
  }
  input::-webkit-search-cancel-button {
    display: none;
  }
  .clear {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .clear:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: var(--space-3) var(--space-2);
  }
  .hint,
  .status {
    margin: 0 6px 10px;
    color: var(--text-faint);
    font-size: var(--text-xs);
    line-height: 1.5;
  }
  .status:empty {
    display: none;
  }
  h2 {
    margin: 18px 6px 8px;
    font-size: 11.5px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding: 0 6px;
  }
  .tag {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 26px;
    padding: 0 9px;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--bg);
    color: var(--text-muted);
    font: inherit;
    font-size: var(--text-xs);
    cursor: pointer;
  }
  .tag:hover {
    border-color: var(--accent);
    color: var(--accent-strong);
  }
  .n {
    color: var(--text-faint);
  }
  .hits button {
    display: flex;
    flex-direction: column;
    gap: 3px;
    width: 100%;
    padding: 8px 10px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .hits button.active {
    background: var(--bg-hover);
  }
  .title {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  .name {
    overflow: hidden;
    font-size: var(--text-sm);
    font-weight: 600;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .folder {
    flex: none;
    max-width: 50%;
    overflow: hidden;
    color: var(--text-faint);
    font-size: var(--text-xs);
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .snippet {
    display: -webkit-box;
    overflow: hidden;
    color: var(--text-muted);
    font-size: var(--text-xs);
    line-height: 1.5;
    overflow-wrap: anywhere;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .snippet.faint {
    color: var(--text-faint);
  }
  mark {
    padding: 0 1px;
    border-radius: 3px;
    background: var(--accent-soft);
    color: var(--accent-strong);
    font-weight: 600;
  }
  .hit-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    color: var(--accent);
    font-size: var(--text-xs);
  }
  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
