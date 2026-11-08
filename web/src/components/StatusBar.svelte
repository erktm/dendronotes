<script lang="ts">
  import Peers from './Peers.svelte'
  import type { NoteSession } from '../lib/state/note.svelte'
  import { settings } from '../lib/state/settings.svelte'
  import { editorStatus } from '../lib/state/status.svelte'
  import { strings } from '../lib/strings'

  let { session }: { session: NoteSession } = $props()

  const words = $derived(session.content.match(/[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*/gu)?.length ?? 0)
  const editing = $derived(settings.view !== 'read')
</script>

<footer class="statusbar">
  <Peers peers={session.peers} />
  <span class="spacer"></span>
  {#if editing}
    <span>{strings.editor.lineCol(editorStatus.line, editorStatus.col)}</span>
  {/if}
  <span>{strings.editor.words(words)}</span>
  {#if editing && editorStatus.vimMode}
    <span class="vim">{editorStatus.vimMode.toUpperCase()}</span>
  {/if}
</footer>

<style>
  .statusbar {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--space-4);
    height: 32px;
    padding: 0 var(--space-4);
    border-top: 1px solid var(--line);
    color: var(--text-faint);
    font-size: var(--text-xs);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .spacer {
    flex: 1;
  }
  .vim {
    padding: 1px 6px;
    border-radius: 4px;
    background: var(--accent);
    color: var(--on-accent);
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 600;
  }
</style>
