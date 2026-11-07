<script lang="ts">
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { fuzzyMatch, highlightChunks } from '../lib/fuzzy'
  import { dirname, displayName, nameProblem } from '../lib/paths'
  import { strings } from '../lib/strings'

  type Item =
    | { kind: 'note'; path: string; dir: string; chunks: { text: string; hit: boolean }[] }
    | { kind: 'create'; name: string }

  let dialog: HTMLDialogElement | undefined = $state()
  let input: HTMLInputElement | undefined = $state()
  let query = $state('')
  let selected = $state(0)

  // New notes go next to the open note.
  const folder = $derived(app.note ? dirname(app.note.path) : '')

  const items = $derived.by((): Item[] => {
    const q = query.trim()
    const notes = app.notePaths
    let ranked: { path: string; score: number; hits: number[] }[]
    if (!q) {
      ranked = notes.slice(0, 50).map((path) => ({ path, score: 0, hits: [] }))
    } else {
      ranked = []
      for (const path of notes) {
        const byName = fuzzyMatch(q, displayName(path))
        if (byName) {
          ranked.push({ path, score: byName.score + 5, hits: byName.indices })
          continue
        }
        const byPath = fuzzyMatch(q, path.replace(/\.md$/i, ''))
        if (byPath) ranked.push({ path, score: byPath.score, hits: [] })
      }
      ranked.sort((a, b) => b.score - a.score)
      ranked = ranked.slice(0, 50)
    }
    const out: Item[] = ranked.map((r) => ({
      kind: 'note',
      path: r.path,
      dir: dirname(r.path),
      chunks: highlightChunks(displayName(r.path), r.hits),
    }))
    const exists = notes.some((p) => displayName(p).toLowerCase() === q.toLowerCase())
    if (q && !exists && !nameProblem(q) && app.canEdit) out.push({ kind: 'create', name: q })
    return out
  })

  $effect(() => {
    if (!dialog) return
    if (app.switcherOpen && !dialog.open) {
      query = ''
      selected = 0
      dialog.showModal()
      input?.focus()
    } else if (!app.switcherOpen && dialog.open) {
      dialog.close()
    }
  })

  $effect(() => {
    void query
    selected = 0
  })

  $effect(() => {
    document.getElementById(`switcher-option-${selected}`)?.scrollIntoView({ block: 'nearest' })
  })

  function close() {
    app.switcherOpen = false
  }

  function choose(index: number, forceCreate = false) {
    const q = query.trim()
    if (forceCreate) {
      if (q && !nameProblem(q)) {
        close()
        void app.createEntry('note', folder, q)
      }
      return
    }
    const item = items[index]
    if (!item) return
    close()
    if (item.kind === 'note') void app.navigate(item.path)
    else void app.createEntry('note', folder, item.name)
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      selected = Math.min(selected + 1, items.length - 1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      selected = Math.max(selected - 1, 0)
    } else if (e.key === 'Enter') {
      e.preventDefault()
      choose(selected, e.shiftKey)
    }
  }

  /** Clicking the backdrop (outside the panel) closes the switcher. */
  function backdropClose(node: HTMLDialogElement) {
    const onclick = (e: MouseEvent) => {
      if (e.target === node) close()
    }
    node.addEventListener('click', onclick)
    return { destroy: () => node.removeEventListener('click', onclick) }
  }
</script>

<dialog bind:this={dialog} aria-label={strings.switcher.title} onclose={close} use:backdropClose>
  <div class="panel">
    <label class="search">
      <Icon name="search" size={18} />
      <input
        bind:this={input}
        bind:value={query}
        {onkeydown}
        type="text"
        role="combobox"
        aria-expanded="true"
        aria-controls="switcher-list"
        aria-activedescendant={items.length ? `switcher-option-${selected}` : undefined}
        aria-label={strings.switcher.placeholder}
        placeholder={strings.switcher.placeholder}
        autocomplete="off"
        spellcheck="false"
      />
    </label>
    <div class="results" id="switcher-list" role="listbox" tabindex="-1" aria-label={strings.switcher.results}>
      {#each items as item, i (item.kind === 'note' ? item.path : `create:${item.name}`)}
        <button
          type="button"
          role="option"
          id="switcher-option-{i}"
          class:sel={i === selected}
          aria-selected={i === selected}
          tabindex="-1"
          onclick={() => choose(i)}
          onmousemove={() => (selected = i)}
        >
          {#if item.kind === 'note'}
            <Icon name="note" size={17} />
            <span class="name">{#each item.chunks as c, ci (ci)}{#if c.hit}<b>{c.text}</b>{:else}{c.text}{/if}{/each}</span>
            <span class="path">{item.dir}</span>
          {:else}
            <Icon name="plus" size={17} />
            <span class="name create">{strings.switcher.create(item.name)}</span>
            <span class="path">{strings.switcher.inFolder(folder)}</span>
          {/if}
        </button>
      {:else}
        <p class="empty">{strings.switcher.empty}</p>
      {/each}
    </div>
    <footer>
      <span><kbd>↑</kbd> <kbd>↓</kbd> {strings.switcher.navigate}</span>
      <span><kbd>↵</kbd> {strings.switcher.openKey}</span>
      <span><kbd>⇧</kbd> <kbd>↵</kbd> {strings.switcher.createKey}</span>
      <span><kbd>esc</kbd> {strings.switcher.close}</span>
    </footer>
  </div>
</dialog>

<style>
  dialog {
    width: min(600px, calc(100vw - 32px));
    max-height: none;
    margin: 12vh auto auto;
    padding: 0;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow), 0 24px 64px rgb(0 0 0 / 0.22);
  }
  dialog::backdrop {
    background: var(--scrim);
  }
  .search {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 52px;
    padding: 0 16px;
    border-bottom: 1px solid var(--line);
    color: var(--text-muted);
  }
  input {
    flex: 1;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 16px;
  }
  .results {
    max-height: min(420px, 50vh);
    overflow: auto;
    padding: 6px;
  }
  .results button {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 8px 10px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font-size: 14px;
    text-align: left;
    cursor: pointer;
  }
  .results button :global(.icon) {
    color: var(--text-faint);
  }
  .results button.sel {
    background: var(--accent-soft);
  }
  .results button.sel :global(.icon) {
    color: var(--accent);
  }
  .name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .name b {
    color: var(--accent-strong);
    font-weight: 650;
  }
  .create {
    color: var(--accent);
    font-weight: 550;
  }
  .path {
    flex: none;
    margin-left: auto;
    color: var(--text-faint);
    font-size: 12px;
  }
  .empty {
    margin: 0;
    padding: 12px 10px;
    color: var(--text-faint);
  }
  footer {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 16px;
    padding: 8px 16px;
    border-top: 1px solid var(--line);
    color: var(--text-faint);
    font-size: 12px;
  }
  kbd {
    padding: 0 5px;
    border: 1px solid var(--line-strong);
    border-radius: 4px;
    background: var(--bg);
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: 11px;
  }
</style>
