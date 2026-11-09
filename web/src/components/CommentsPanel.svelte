<script lang="ts">
  import Avatar from './Avatar.svelte'
  import Icon from './Icon.svelte'
  import MentionInput from './MentionInput.svelte'
  import type { NoteSession } from '../lib/state/note.svelte'
  import { app } from '../lib/state/app.svelte'
  import { comments, type Thread } from '../lib/state/comments.svelte'
  import * as api from '../lib/api/comments'
  import type { Comment } from '../lib/api/comments'
  import { findAnchor } from '../lib/anchors'
  import { parts } from '../lib/commentText'
  import { errorMessage } from '../lib/errors'
  import { rank } from '../lib/roles'
  import { ago } from '../lib/time'
  import { strings } from '../lib/strings'

  let { session }: { session: NoteSession } = $props()

  const t = strings.comments

  let draftText = $state('')
  let replyTo = $state<number | null>(null)
  let replyText = $state('')
  let editing = $state<number | null>(null)
  let editText = $state('')
  let showResolved = $state(false)
  let busy = $state(false)

  const me = $derived(app.user?.id ?? 0)
  const role = $derived(app.soma?.role)

  /** Threads in the order of their passages; on the whole note and detached ones after. */
  const ordered = $derived.by(() => {
    const text = session.content
    const placed = comments.threads.map((th) => {
      const span = th.root.quote ? findAnchor(text, th.root) : null
      return { th, pos: span ? span.from : th.root.quote ? Infinity : Number.MAX_SAFE_INTEGER, detached: !!th.root.quote && !span }
    })
    placed.sort((a, b) => a.pos - b.pos || a.th.root.id - b.th.root.id)
    return placed
  })
  const open = $derived(ordered.filter((p) => !p.th.root.resolvedAt))
  const resolved = $derived(ordered.filter((p) => p.th.root.resolvedAt))

  const nameOf = (id: number) => {
    const p = comments.person(id)
    return p ? p.displayName || p.username : t.unknown
  }
  const canDelete = (c: Comment) => c.authorId === me || rank(role) >= rank('editor')
  const shortQuote = (q: string) => (q.length > 160 ? `${q.slice(0, 157)}…` : q)

  async function run(fn: () => Promise<unknown>): Promise<boolean> {
    busy = true
    try {
      await fn()
      await comments.load(session.path)
      return true
    } catch (err) {
      app.toast(errorMessage(err))
      return false
    } finally {
      busy = false
    }
  }

  async function submitDraft() {
    const body = draftText.trim()
    if (!body || busy) return
    const anchor = comments.draft
    const ok = await run(() => api.addComment({ path: session.path, body, ...(anchor ?? {}) }))
    if (ok) {
      draftText = ''
      comments.draft = null
    }
  }

  async function submitReply(th: Thread) {
    const body = replyText.trim()
    if (!body || busy) return
    if (await run(() => api.addComment({ threadId: th.root.id, body }))) {
      replyText = ''
      replyTo = null
    }
  }

  async function saveEdit(c: Comment) {
    const body = editText.trim()
    if (!body || busy) return
    if (await run(() => api.editComment(c.id, body))) editing = null
  }

  async function remove(c: Comment) {
    const ok = await app.ask({
      title: t.deleteTitle,
      message: c.threadId === 0 ? t.deleteThreadText : t.deleteText,
      confirmLabel: t.delete,
      danger: true,
    })
    if (ok) await run(() => api.deleteComment(c.id))
  }

  const resolve = (th: Thread, done: boolean) => run(() => api.resolveThread(th.root.id, done))

  function startReply(th: Thread) {
    replyTo = th.root.id
    replyText = ''
    comments.active = th.root.id
  }

  function startEdit(c: Comment) {
    editing = c.id
    editText = c.body
  }

  function newOnNote() {
    comments.draft = null
    comments.draftFocus++
  }
</script>

{#snippet body(c: Comment)}
  <p class="text">
    {#each parts(c.body) as p, i (i)}
      {#if p.kind === 'mention'}<span class="mention">{p.text}</span>
      {:else if p.kind === 'link'}<a href={p.href} target="_blank" rel="noopener noreferrer nofollow">{p.text}</a>
      {:else}{p.text}{/if}
    {/each}
  </p>
{/snippet}

{#snippet comment(c: Comment)}
  {@const author = comments.person(c.authorId)}
  <div class="comment">
    <div class="meta">
      {#if author}
        <Avatar id={author.id} name={author.displayName || author.username} avatar={author.avatar} size={22} />
      {:else}
        <span class="ghost" aria-hidden="true"><Icon name="user" size={13} /></span>
      {/if}
      <strong>{nameOf(c.authorId)}</strong>
      <span class="when">{ago(c.createdAt)}{#if c.editedAt} · {t.edited}{/if}</span>
      <span class="tools" role="group" aria-label={t.actionsOf(nameOf(c.authorId))}>
        {#if c.authorId === me && editing !== c.id}
          <button type="button" class="mini" aria-label={t.edit} title={t.edit} onclick={() => startEdit(c)}>
            <Icon name="pencil" size={13} />
          </button>
        {/if}
        {#if canDelete(c)}
          <button type="button" class="mini" aria-label={t.delete} title={t.delete} disabled={busy} onclick={() => void remove(c)}>
            <Icon name="trash" size={13} />
          </button>
        {/if}
      </span>
    </div>
    {#if editing === c.id}
      <MentionInput
        bind:value={editText}
        label={t.edit}
        placeholder={t.placeholder}
        focus={1}
        onsubmit={() => void saveEdit(c)}
        oncancel={() => (editing = null)}
      />
      <div class="row">
        <button type="button" class="btn" onclick={() => (editing = null)}>{t.cancel}</button>
        <button type="button" class="btn primary" disabled={busy || !editText.trim()} onclick={() => void saveEdit(c)}>{t.save}</button>
      </div>
    {:else}
      {@render body(c)}
    {/if}
  </div>
{/snippet}

{#snippet thread(th: Thread, detached: boolean)}
  <li class="thread" class:active={comments.active === th.root.id} class:done={!!th.root.resolvedAt}>
    {#if th.root.quote}
      <button
        type="button"
        class="quote"
        class:detached
        title={detached ? t.detached : t.showInNote}
        disabled={detached}
        onclick={() => comments.show(th.root.id)}
      >
        “{shortQuote(th.root.quote)}”
      </button>
      {#if detached}<p class="hint">{t.detached}</p>{/if}
    {:else}
      <p class="hint">{t.wholeNote}</p>
    {/if}
    {@render comment(th.root)}
    {#each th.replies as r (r.id)}
      {@render comment(r)}
    {/each}
    {#if th.root.resolvedAt}
      <p class="hint">{t.resolvedBy(nameOf(th.root.resolvedBy ?? 0), ago(th.root.resolvedAt))}</p>
    {/if}
    {#if replyTo === th.root.id}
      <MentionInput
        bind:value={replyText}
        label={t.reply}
        placeholder={t.replyPlaceholder}
        focus={th.root.id}
        onsubmit={() => void submitReply(th)}
        oncancel={() => (replyTo = null)}
      />
      <div class="row">
        <button type="button" class="btn" onclick={() => (replyTo = null)}>{t.cancel}</button>
        <button type="button" class="btn primary" disabled={busy || !replyText.trim()} onclick={() => void submitReply(th)}>{t.reply}</button>
      </div>
    {:else}
      <div class="row">
        {#if app.can.write}
          {#if th.root.resolvedAt}
            <button type="button" class="btn" disabled={busy} onclick={() => void resolve(th, false)}>{t.reopen}</button>
          {:else}
            <button type="button" class="btn" title={t.resolveHint} disabled={busy} onclick={() => void resolve(th, true)}>
              <Icon name="check" size={14} />
              {t.resolve}
            </button>
          {/if}
        {/if}
        <button type="button" class="btn" onclick={() => startReply(th)}>{t.reply}</button>
      </div>
    {/if}
  </li>
{/snippet}

<aside class="panel" aria-label={t.title}>
  <header>
    <h2>{t.title}</h2>
    <button type="button" class="mini" aria-label={t.close} title={t.close} onclick={() => (app.commentsOpen = false)}>
      <Icon name="close" size={16} />
    </button>
  </header>

  <div class="new" class:on={!!comments.draft}>
    {#if comments.draft}
      <p class="quote static">“{shortQuote(comments.draft.quote)}”</p>
    {/if}
    <MentionInput
      bind:value={draftText}
      label={comments.draft ? t.add : t.addOnNote}
      placeholder={t.placeholder}
      focus={comments.draftFocus}
      onsubmit={() => void submitDraft()}
      oncancel={() => ((comments.draft = null), (draftText = ''))}
    />
    <div class="row">
      <span class="hint">{t.sendHint}</span>
      {#if comments.draft}
        <button type="button" class="btn" onclick={newOnNote}>{t.cancel}</button>
      {/if}
      <button type="button" class="btn primary" disabled={busy || !draftText.trim()} onclick={() => void submitDraft()}>{t.send}</button>
    </div>
  </div>

  {#if !comments.loaded}
    <p class="empty">{t.loading}</p>
  {:else if comments.threads.length === 0}
    <p class="empty">{t.none}</p>
  {:else}
    <ul class="threads">
      {#each open as p (p.th.root.id)}
        {@render thread(p.th, p.detached)}
      {/each}
    </ul>
    {#if resolved.length}
      <button type="button" class="toggle" aria-expanded={showResolved} onclick={() => (showResolved = !showResolved)}>
        <Icon name="chevron" size={14} />
        {t.resolvedList(resolved.length)}
      </button>
      {#if showResolved}
        <ul class="threads">
          {#each resolved as p (p.th.root.id)}
            {@render thread(p.th, p.detached)}
          {/each}
        </ul>
      {/if}
    {/if}
  {/if}
</aside>

<style>
  .panel {
    display: flex;
    flex: none;
    flex-direction: column;
    width: 320px;
    min-height: 0;
    overflow: auto;
    border-left: 1px solid var(--line);
    background: var(--bg-raised);
    font-size: var(--text-sm);
  }
  header {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: space-between;
    padding: 12px 10px 8px 18px;
  }
  h2 {
    font-size: 11.5px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .new {
    padding: 4px 14px 14px;
    border-bottom: 1px solid var(--line);
  }
  .row {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 6px;
  }
  .row .hint {
    margin: 0 auto 0 0;
  }
  .hint,
  .when {
    color: var(--text-faint);
    font-size: var(--text-xs);
  }
  p.hint {
    margin: 0 0 6px;
  }
  .empty {
    padding: 18px;
    color: var(--text-faint);
  }
  .threads {
    margin: 0;
    padding: 8px 10px;
    list-style: none;
  }
  .thread {
    padding: 10px;
    border: 1px solid var(--line);
    border-radius: var(--radius);
    background: var(--bg);
  }
  .thread + .thread {
    margin-top: 8px;
  }
  .thread.active {
    border-color: color-mix(in srgb, var(--warn) 60%, var(--line));
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--warn) 35%, transparent);
  }
  .thread.done {
    opacity: 0.8;
  }
  .quote {
    display: block;
    width: 100%;
    margin: 0 0 8px;
    padding: 2px 0 2px 8px;
    border: 0;
    border-left: 3px solid color-mix(in srgb, var(--warn) 60%, transparent);
    background: transparent;
    color: var(--text-muted);
    font: inherit;
    font-size: var(--text-xs);
    font-style: italic;
    text-align: left;
    overflow-wrap: anywhere;
    cursor: pointer;
  }
  .quote.detached {
    border-left-style: dashed;
    text-decoration: line-through;
    cursor: default;
  }
  .quote.static {
    cursor: default;
  }
  .comment + .comment {
    margin-top: 10px;
    padding-top: 10px;
    border-top: 1px solid var(--line);
  }
  .meta {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }
  .meta strong {
    overflow: hidden;
    font-size: var(--text-sm);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ghost {
    display: grid;
    flex: none;
    place-items: center;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    background: var(--bg-sunken);
    color: var(--text-faint);
  }
  .tools {
    display: flex;
    gap: 2px;
    margin-left: auto;
    opacity: 0;
  }
  .comment:hover .tools,
  .comment:focus-within .tools {
    opacity: 1;
  }
  .text {
    margin: 4px 0 0 28px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    line-height: 1.5;
  }
  .mention {
    color: var(--accent-strong);
    font-weight: 600;
  }
  .text a {
    color: var(--accent-strong);
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 26px;
    padding: 0 9px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    color: var(--text);
    font-size: var(--text-xs);
    font-weight: 600;
    cursor: pointer;
  }
  .btn:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .btn.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--on-accent);
  }
  .btn:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .mini {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .mini:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 6px;
    margin: 4px 10px 12px;
    padding: 4px 8px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    font-size: var(--text-xs);
    font-weight: 600;
    cursor: pointer;
  }
  .toggle[aria-expanded='true'] :global(svg) {
    transform: rotate(90deg);
  }
  @media (max-width: 1100px) {
    .panel {
      position: fixed;
      z-index: 20;
      top: 48px;
      right: 0;
      bottom: 0;
      width: min(92vw, 340px);
      box-shadow: var(--shadow);
    }
  }
</style>
