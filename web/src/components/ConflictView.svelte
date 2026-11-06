<script lang="ts">
  import Icon from './Icon.svelte'
  import type { NoteSession } from '../lib/state/note.svelte'
  import { diffLines, type DiffLine } from '../lib/diff'
  import { strings } from '../lib/strings'

  let { session }: { session: NoteSession } = $props()

  $effect(() => {
    if (session.conflict !== null && session.disk === null) void session.loadDisk()
  })

  // Both sides are compared with the version the user started from.
  const mine = $derived(diffLines(session.saved, session.content))
  const disk = $derived(session.disk === null ? null : diffLines(session.saved, session.disk))

  const sign = (l: DiffLine) => (l.type === 'add' ? '+' : l.type === 'del' ? '−' : ' ')

  // The clicked button disappears with the comparison; the editor that
  // replaces it takes the focus, so typing can continue right away.
  function useDisk() {
    session.autofocus = true
    void session.reloadFromDisk()
  }
  function keepMine() {
    session.autofocus = true
    void session.save(true)
  }
</script>

<div class="conflict">
  <div class="banner" role="alert">
    <Icon name="alert" size={18} />
    <p><strong>{strings.note.conflictTitle}</strong> {strings.note.conflictText} {strings.conflict.text}</p>
    <button type="button" class="btn" onclick={useDisk}>{strings.note.useDisk}</button>
    <button type="button" class="btn primary" disabled={session.disk === null} onclick={keepMine}>
      {strings.note.keepMine}
    </button>
  </div>

  <div class="compare">
    <section class="pane" aria-label={strings.conflict.yours}>
      <header>{strings.conflict.yours} <span>{strings.conflict.yoursHint}</span></header>
      <pre>{#each mine as l, i (i)}<span class="l {l.type}"><span class="sign" aria-hidden="true">{sign(l)}</span>{l.text}</span>{/each}</pre>
    </section>
    <section class="pane" aria-label={strings.conflict.disk}>
      <header>{strings.conflict.disk} <span>{strings.conflict.diskHint}</span></header>
      {#if disk}
        <pre>{#each disk as l, i (i)}<span class="l {l.type}"><span class="sign" aria-hidden="true">{sign(l)}</span>{l.text}</span>{/each}</pre>
      {:else}
        <p class="loading">{strings.conflict.loading}</p>
      {/if}
    </section>
  </div>
  <p class="explain">{strings.conflict.explain}</p>
</div>

<style>
  .conflict {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  .banner {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px 14px;
    padding: 12px 24px;
    border-bottom: 1px solid var(--line);
    background: var(--warn-soft);
    font-size: 13.5px;
  }
  .banner :global(.icon) {
    color: var(--warn-text);
  }
  .banner p {
    flex: 1;
    min-width: 20ch;
    margin: 0;
  }
  .btn {
    height: 32px;
    padding: 0 14px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    font-size: 13px;
    font-weight: 550;
    cursor: pointer;
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
  .compare {
    display: grid;
    flex: 1;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 16px;
    min-height: 0;
    padding: 20px 24px 0;
  }
  .pane {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--bg-raised);
  }
  .pane header {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--line);
    font-size: 13px;
    font-weight: 600;
  }
  .pane header span {
    color: var(--text-faint);
    font-size: 12px;
    font-weight: 400;
  }
  pre {
    flex: 1;
    margin: 0;
    padding: 10px 0;
    overflow: auto;
    font-family: var(--font-mono);
    font-size: 13px;
    line-height: 1.75;
  }
  .l {
    display: block;
    min-height: 1.75em;
    padding: 0 14px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .sign {
    display: inline-block;
    width: 16px;
    color: var(--text-faint);
  }
  .add {
    background: var(--add-bg);
  }
  .del {
    background: var(--del-bg);
  }
  .loading,
  .explain {
    margin: 0;
    padding: 12px 14px;
    color: var(--text-muted);
    font-size: 12.5px;
  }
  .explain {
    padding: 12px 24px 16px;
  }
  @media (max-width: 760px) {
    .compare {
      grid-template-columns: minmax(0, 1fr);
      overflow: auto;
    }
  }
</style>
