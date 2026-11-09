<script lang="ts">
  import Avatar from './Avatar.svelte'
  import { somaPeople } from '../lib/api/comments'
  import type { Person } from '../lib/api/soma'
  import { mentionAt } from '../lib/commentText'
  import { strings } from '../lib/strings'

  let {
    value = $bindable(''),
    placeholder,
    label,
    focus = 0,
    onsubmit,
    oncancel,
  }: {
    value?: string
    placeholder: string
    label: string
    /** Bumped to move the focus here. */
    focus?: number
    onsubmit: () => void
    oncancel?: () => void
  } = $props()

  let area: HTMLTextAreaElement | undefined = $state()
  let people = $state.raw<Person[]>([])
  let at = $state<{ from: number; query: string } | null>(null)
  let selected = $state(0)
  let timer: ReturnType<typeof setTimeout> | undefined
  const listId = `mentions-${Math.random().toString(36).slice(2, 8)}`

  $effect(() => {
    if (focus && area) area.focus()
  })

  // Grows with the text, up to a limit.
  $effect(() => {
    void value
    if (!area) return
    area.style.height = 'auto'
    area.style.height = `${Math.min(area.scrollHeight, 220)}px`
  })

  function lookup() {
    if (!area) return
    at = mentionAt(value, area.selectionStart)
    clearTimeout(timer)
    if (!at) {
      people = []
      return
    }
    const query = at.query
    timer = setTimeout(async () => {
      try {
        const found = await somaPeople(query)
        if (at?.query === query) {
          people = found
          selected = 0
        }
      } catch {
        people = []
      }
    }, 120)
  }

  function choose(p: Person) {
    if (!at || !area) return
    const insert = `@${p.username} `
    const end = at.from + 1 + at.query.length
    value = value.slice(0, at.from) + insert + value.slice(end)
    const cursor = at.from + insert.length
    people = []
    at = null
    queueMicrotask(() => {
      area?.focus()
      area?.setSelectionRange(cursor, cursor)
    })
  }

  function keydown(e: KeyboardEvent) {
    if (people.length && at) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault()
        selected = (selected + (e.key === 'ArrowDown' ? 1 : people.length - 1)) % people.length
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        const p = people[selected]
        if (p) choose(p)
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        people = []
        at = null
        return
      }
    }
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      onsubmit()
    } else if (e.key === 'Escape' && oncancel) {
      e.preventDefault()
      e.stopPropagation()
      oncancel()
    }
  }
</script>

<div class="mention-input">
  <textarea
    bind:this={area}
    bind:value
    rows="2"
    maxlength="5000"
    {placeholder}
    aria-label={label}
    aria-autocomplete="list"
    aria-controls={people.length ? listId : undefined}
    oninput={lookup}
    onclick={lookup}
    onkeydown={keydown}
    onblur={() => setTimeout(() => (people = []), 150)}
  ></textarea>
  {#if people.length && at}
    <ul class="people" id={listId} role="listbox" aria-label={strings.comments.people}>
      {#each people as p, i (p.id)}
        <li role="option" aria-selected={i === selected}>
          <button type="button" tabindex="-1" onmousedown={(e) => (e.preventDefault(), choose(p))}>
            <Avatar id={p.id} name={p.displayName || p.username} avatar={p.avatar} size={20} />
            <span class="name">{p.displayName || p.username}</span>
            <span class="user">@{p.username}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .mention-input {
    position: relative;
  }
  textarea {
    display: block;
    width: 100%;
    min-height: 54px;
    padding: 7px 9px;
    resize: none;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
    line-height: 1.45;
  }
  textarea:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
    border-color: transparent;
  }
  .people {
    position: absolute;
    z-index: 5;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    margin: 0;
    padding: 4px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    box-shadow: var(--shadow);
    list-style: none;
  }
  .people button {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 5px 6px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--text);
    font-size: var(--text-sm);
    text-align: left;
    cursor: pointer;
  }
  [aria-selected='true'] button,
  .people button:hover {
    background: var(--bg-hover);
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .user {
    margin-left: auto;
    color: var(--text-faint);
    font-size: var(--text-xs);
  }
</style>
