<script lang="ts">
  import type { NoteSession } from '../lib/state/note.svelte'
  import { app } from '../lib/state/app.svelte'
  import { live } from '../lib/state/live.svelte'
  import { backlinks as fetchBacklinks, type Backlink } from '../lib/api/soma'
  import { displayName } from '../lib/paths'
  import { outline } from '../lib/outline'
  import { parseSynapses, synapseLabel } from '../lib/synapses'
  import { strings } from '../lib/strings'

  let { session }: { session: NoteSession } = $props()

  const headings = $derived(outline(session.content))

  /** One entry per distinct target, in order of first appearance. */
  const synapses = $derived.by(() => {
    const seen = new Set<string>()
    const out: { target: string; markdown: boolean; label: string; resolved: string | null; embed: boolean }[] = []
    for (const ref of parseSynapses(session.content)) {
      const resolved = app.resolveSynapse(ref.target, ref.markdown, session.path)
      const key = resolved ?? `?${ref.target.toLowerCase()}`
      if (seen.has(key)) continue
      seen.add(key)
      out.push({ target: ref.target, markdown: ref.markdown, label: synapseLabel({ target: ref.target }), resolved, embed: ref.embed })
    }
    return out
  })

  /** Notes with synapses to this one; null while loading. */
  let incoming = $state.raw<Backlink[] | null>(null)
  let incomingFor = ''

  $effect(() => {
    const path = session.path
    void live.version // any change in the soma may add or remove synapses
    if (path !== incomingFor) incoming = null
    const ctrl = new AbortController()
    const timer = setTimeout(
      async () => {
        try {
          incoming = await fetchBacklinks(path, ctrl.signal)
          incomingFor = path
        } catch {
          if (!ctrl.signal.aborted) incoming = []
        }
      },
      path === incomingFor ? 300 : 0,
    )
    return () => {
      clearTimeout(timer)
      ctrl.abort()
    }
  })

  const incomingCount = $derived(incoming?.reduce((n, b) => n + b.synapses.length, 0) ?? 0)
</script>

<aside class="panel" aria-label={strings.details.label}>
  <section>
    <h2>{strings.details.outline}</h2>
    {#if headings.length === 0}
      <p class="empty">{strings.details.noHeadings}</p>
    {:else}
      <ol class="outline">
        {#each headings as h (h.line)}
          <li class="level-{Math.min(h.level, 4)}">
            <button type="button" onclick={() => (app.scrollRequest = { line: h.line, slug: h.slug })}>{h.text}</button>
          </li>
        {/each}
      </ol>
    {/if}
  </section>

  <section>
    <h2>{strings.details.outgoing} <span class="count">{synapses.length}</span></h2>
    {#if synapses.length === 0}
      <p class="empty">{strings.details.noSynapses}</p>
    {:else}
      <ul class="synapses">
        {#each synapses as s (s.resolved ?? s.target)}
          <li>
            <button
              type="button"
              class:unresolved={!s.resolved}
              onclick={() => void app.openSynapse(s.target, s.markdown, session.path)}
            >
              <span class="dot" aria-hidden="true"></span>
              <span class="label">{s.label}</span>
              {#if !s.resolved}<span class="hint">· {strings.details.notCreated}</span>{:else if s.embed}<span class="hint">· {strings.details.embed}</span>{/if}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section>
    <h2>{strings.details.incoming} {#if incoming}<span class="count">{incomingCount}</span>{/if}</h2>
    {#if incoming === null}
      <p class="empty">{strings.details.incomingLoading}</p>
    {:else if incoming.length === 0}
      <p class="empty">{strings.details.noIncoming}</p>
    {:else}
      <ul class="incoming">
        {#each incoming as b (b.path)}
          <li>
            <button type="button" onclick={() => void app.navigate(b.path)}>
              <span class="dot" aria-hidden="true"></span>
              <span class="label">{displayName(b.path)}</span>
            </button>
            <ul class="contexts">
              {#each b.synapses as syn, i (i)}
                <li>
                  <button
                    type="button"
                    class="context"
                    title={strings.details.openAt(displayName(b.path), syn.line)}
                    onclick={() => void app.openAt(b.path, syn.line)}
                  >
                    {syn.context}
                  </button>
                </li>
              {/each}
            </ul>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</aside>

<style>
  .panel {
    width: 280px;
    flex: none;
    overflow: auto;
    padding: 20px 14px;
    border-left: 1px solid var(--line);
    background: var(--bg-raised);
    font-size: var(--text-sm);
  }
  section + section {
    margin-top: 24px;
  }
  h2 {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 8px 6px;
    font-size: 11.5px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .count {
    padding: 0 6px;
    border-radius: 999px;
    background: var(--bg-sunken);
    font-size: 11px;
    letter-spacing: 0;
  }
  .empty {
    margin: 0 0 0 6px;
    color: var(--text-faint);
  }
  ol,
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  button {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    min-height: 28px;
    padding: 3px 8px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text-muted);
    font-size: var(--text-sm);
    text-align: left;
    cursor: pointer;
  }
  button:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .level-2 button {
    padding-left: 20px;
  }
  .level-3 button {
    padding-left: 32px;
  }
  .level-4 button {
    padding-left: 44px;
  }
  .dot {
    width: 7px;
    height: 7px;
    flex: none;
    border-radius: 50%;
    background: var(--accent);
  }
  .unresolved .dot {
    border: 1px dashed var(--accent-2);
    background: transparent;
  }
  .label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .hint {
    flex: none;
    color: var(--text-faint);
  }
  .contexts {
    margin: 0 0 6px 15px;
    padding-left: 8px;
    border-left: 1px solid var(--line);
  }
  .context {
    display: -webkit-box;
    min-height: 0;
    padding: 4px 8px;
    overflow: hidden;
    color: var(--text-faint);
    font-size: var(--text-xs);
    line-height: 1.45;
    white-space: normal;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  @media (max-width: 1100px) {
    .panel {
      display: none;
    }
  }
</style>
