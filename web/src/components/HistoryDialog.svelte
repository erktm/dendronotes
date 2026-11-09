<script lang="ts">
  import { closeOnBackdrop } from '../lib/actions'
  import Icon from './Icon.svelte'
  import Avatar from './Avatar.svelte'
  import { app } from '../lib/state/app.svelte'
  import { live } from '../lib/state/live.svelte'
  import * as api from '../lib/api/soma'
  import type { Person, Version } from '../lib/api/soma'
  import { errorMessage } from '../lib/errors'
  import { diffLines, stats, withContext } from '../lib/diff'
  import { displayName } from '../lib/paths'
  import { ago, stamp } from '../lib/time'
  import { strings } from '../lib/strings'

  const t = strings.history

  let dialog: HTMLDialogElement | undefined = $state()
  let path = $state('')
  let versions = $state.raw<Version[] | null>(null)
  let people = $state.raw<Record<string, Person>>({})
  let selected = $state<number | null>(null)
  let content = $state.raw<{ id: number; text: string } | null>(null)
  let busy = $state(false)

  $effect(() => {
    if (!dialog) return
    if (app.historyOpen && app.note && !dialog.open) {
      path = app.note.path
      versions = null
      selected = null
      content = null
      dialog.showModal()
    } else if ((!app.historyOpen || !app.note) && dialog.open) dialog.close()
  })

  // Reload while open: a save adds or updates a version.
  $effect(() => {
    void live.version
    if (app.historyOpen && path) void load()
  })

  async function load() {
    try {
      const h = await api.history(path)
      versions = h.versions
      people = h.people
      if (selected === null || !h.versions.some((v) => v.id === selected)) {
        // The newest version usually is the note as it is: start with the one before.
        const first = h.versions[1] ?? h.versions[0]
        if (first) void pick(first.id)
      }
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function pick(id: number) {
    selected = id
    if (content?.id === id) return
    try {
      const v = await api.version(id)
      if (selected === id) content = { id, text: v.content }
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  const nameOf = (id: number) => {
    const p = people[String(id)]
    return p ? p.displayName || p.username : t.someone
  }
  function authors(v: Version): string {
    const names = v.authors.map(nameOf)
    if (names.length === 0) return ''
    const last = names.pop() ?? ''
    return t.by(names.length ? t.and(names.join(', '), last) : last)
  }

  const current = $derived(app.note?.path === path ? app.note.content : '')
  const ops = $derived(content ? diffLines(content.text, current) : [])
  const change = $derived(stats(ops))
  const rows = $derived(withContext(ops))
  const chosen = $derived(versions?.find((v) => v.id === selected) ?? null)

  async function restore() {
    const v = chosen
    if (!v) return
    const ok = await app.ask({ title: t.restoreTitle, message: t.restoreText(stamp(v.at)), confirmLabel: t.restore })
    if (!ok) return
    busy = true
    try {
      await api.restoreVersion(v.id)
      app.toast(t.restored, 'info')
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }
</script>

<dialog bind:this={dialog} use:closeOnBackdrop aria-labelledby="history-title" onclose={() => (app.historyOpen = false)}>
  <header>
    <h2 id="history-title">{t.title(displayName(path))}</h2>
    <button type="button" class="icon-btn" aria-label={t.close} onclick={() => (app.historyOpen = false)}>
      <Icon name="close" size={18} />
    </button>
  </header>

  {#if versions === null}
    <p class="none">{t.loading}</p>
  {:else if versions.length === 0}
    <p class="none">{t.empty}</p>
  {:else}
    <div class="body">
      <nav class="list" aria-label={t.versions}>
        <ul>
          {#each versions as v, i (v.id)}
            <li>
              <button type="button" class:on={v.id === selected} aria-current={v.id === selected} onclick={() => void pick(v.id)}>
                <span class="when">
                  {stamp(v.at)}
                  {#if i === 0}<span class="badge">{t.newest}</span>{/if}
                </span>
                <span class="why">{t.reasons[v.reason] ?? v.reason} {authors(v)}</span>
                {#if v.authors.length}
                  <span class="faces">
                    {#each v.authors as id (id)}
                      {@const p = people[String(id)]}
                      {#if p}<Avatar id={p.id} name={p.displayName || p.username} avatar={p.avatar} size={20} />{/if}
                    {/each}
                  </span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
        <p class="keep">{t.keepNote}</p>
      </nav>

      <section class="view" aria-live="polite">
        {#if chosen && content?.id === chosen.id}
          <div class="view-head">
            <div>
              <strong>{stamp(chosen.at)}</strong>
              <span class="sub">{ago(chosen.at)} · {t.compare}</span>
            </div>
            {#if change.added || change.deleted}
              <span class="stats">{t.stats(change.added, change.deleted)}</span>
            {/if}
            {#if app.can.write && (change.added || change.deleted)}
              <button type="button" class="btn primary" disabled={busy} onclick={() => void restore()}>
                <Icon name="restore" size={15} />
                {t.restore}
              </button>
            {/if}
          </div>
          {#if !change.added && !change.deleted}
            <p class="none">{t.same}</p>
          {:else}
            <div class="diff">
              {#each rows as row, i (i)}
                {#if row.kind === 'skip'}
                  <div class="skip">{t.skipped(row.count)}</div>
                {:else}
                  <div class="line {row.kind}"><span class="mark" aria-hidden="true">{row.kind === 'add' ? '+' : row.kind === 'del' ? '−' : ''}</span>{row.text || ' '}</div>
                {/if}
              {/each}
            </div>
          {/if}
        {:else}
          <p class="none">{t.loading}</p>
        {/if}
      </section>
    </div>
  {/if}
</dialog>

<style>
  dialog {
    width: min(94vw, 980px);
    height: min(86vh, 760px);
    padding: 0;
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow), 0 24px 64px rgb(0 0 0 / 0.2);
  }
  dialog[open] {
    display: flex;
    flex-direction: column;
  }
  dialog::backdrop {
    background: var(--scrim);
  }
  header {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--space-2);
    padding: 14px 14px 14px 20px;
    border-bottom: 1px solid var(--line);
  }
  h2 {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    font-size: var(--text-lg);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .none {
    padding: 28px 20px 36px;
    color: var(--text-faint);
    text-align: center;
  }
  .body {
    display: grid;
    flex: 1;
    grid-template-columns: 300px minmax(0, 1fr);
    min-height: 0;
  }
  .list {
    display: flex;
    flex-direction: column;
    min-height: 0;
    border-right: 1px solid var(--line);
    background: var(--bg);
  }
  .list ul {
    flex: 1;
    margin: 0;
    padding: 8px;
    overflow: auto;
    list-style: none;
  }
  .list button {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    padding: 8px 10px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    text-align: left;
    cursor: pointer;
  }
  .list button:hover {
    background: var(--bg-hover);
  }
  .list button.on {
    background: var(--accent-soft);
  }
  .when {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .badge {
    padding: 0 6px;
    border-radius: 999px;
    background: var(--bg-sunken);
    color: var(--text-muted);
    font-size: 10.5px;
    font-weight: 600;
  }
  .why,
  .sub,
  .keep {
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  .faces {
    display: flex;
    gap: 3px;
    margin-top: 3px;
  }
  .keep {
    flex: none;
    margin: 0;
    padding: 10px 16px 14px;
    border-top: 1px solid var(--line);
  }
  .view {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }
  .view-head {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--space-3);
    padding: 12px 16px;
    border-bottom: 1px solid var(--line);
  }
  .view-head > div {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }
  .stats {
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }
  .diff {
    flex: 1;
    padding: 8px 0 16px;
    overflow: auto;
    font-family: var(--font-mono);
    font-size: 12.5px;
    line-height: 1.6;
  }
  .line {
    padding: 0 16px 0 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .mark {
    display: inline-block;
    width: 28px;
    color: var(--text-faint);
    text-align: center;
    user-select: none;
  }
  .line.add {
    background: color-mix(in srgb, var(--ok) 14%, transparent);
  }
  .line.add .mark {
    color: var(--ok);
  }
  .line.del {
    background: color-mix(in srgb, var(--err) 12%, transparent);
    color: var(--text-muted);
    text-decoration: line-through;
    text-decoration-color: color-mix(in srgb, var(--err) 50%, transparent);
  }
  .line.del .mark {
    color: var(--err);
  }
  .skip {
    margin: 4px 0;
    padding: 2px 16px 2px 28px;
    background: var(--bg-sunken);
    color: var(--text-faint);
    font-family: var(--font-ui);
    font-size: var(--text-xs);
  }
  .btn {
    display: inline-flex;
    flex: none;
    align-items: center;
    gap: 6px;
    height: 30px;
    padding: 0 10px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    color: var(--text);
    font-size: var(--text-sm);
    font-weight: 550;
    cursor: pointer;
  }
  .btn.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--on-accent);
  }
  .icon-btn {
    display: grid;
    flex: none;
    place-items: center;
    width: 32px;
    height: 32px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  @media (max-width: 720px) {
    .body {
      grid-template-columns: 1fr;
      grid-template-rows: minmax(120px, 34%) minmax(0, 1fr);
    }
    .list {
      border-right: 0;
      border-bottom: 1px solid var(--line);
    }
  }
</style>
