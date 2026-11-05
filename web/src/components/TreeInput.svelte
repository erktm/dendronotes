<script lang="ts">
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { cssVars, focusOnMount } from '../lib/actions'
  import { strings } from '../lib/strings'

  let {
    depth,
    kind,
    initial = '',
    onsubmit,
  }: {
    depth: number
    kind: 'note' | 'folder'
    initial?: string
    onsubmit: (name: string) => Promise<boolean>
  } = $props()

  let input: HTMLInputElement | undefined = $state()
  let busy = false
  let done = false

  async function commit() {
    if (done || busy || !input) return
    const name = input.value.trim()
    if (!name || name === initial) {
      cancel()
      return
    }
    busy = true
    const ok = await onsubmit(name)
    busy = false
    if (ok) {
      done = true
      app.edit = null
    } else {
      input.focus()
      input.select()
    }
  }

  function cancel() {
    done = true
    app.edit = null
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      void commit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      cancel()
    }
  }
</script>

<div class="row" use:cssVars={{ '--depth': depth }}>
  <span class="spacer"></span>
  <Icon name={kind === 'folder' ? 'folder' : 'note'} size={17} />
  <input
    bind:this={input}
    value={initial}
    use:focusOnMount={true}
    {onkeydown}
    onblur={() => void commit()}
    spellcheck="false"
    autocomplete="off"
    aria-label={initial ? strings.tree.newName : kind === 'folder' ? strings.tree.folderName : strings.tree.noteName}
    placeholder={kind === 'folder' ? strings.tree.folderName : strings.tree.noteName}
  />
</div>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 30px;
    padding: 0 var(--space-2) 0 calc(var(--space-2) + var(--depth) * 14px);
    color: var(--text-muted);
  }
  .spacer {
    width: 14px;
  }
  input {
    flex: 1;
    min-width: 0;
    height: 24px;
    padding: 0 6px;
    border: 1px solid var(--accent);
    border-radius: 5px;
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
    outline: none;
  }
</style>
