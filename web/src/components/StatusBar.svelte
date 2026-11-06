<script lang="ts">
  import type { NoteSession } from '../lib/state/note.svelte'
  import { editorStatus } from '../lib/state/status.svelte'
  import { strings } from '../lib/strings'

  let { session }: { session: NoteSession } = $props()

  const words = $derived(session.content.match(/[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*/gu)?.length ?? 0)
</script>

<footer class="statusbar">
  <span>{strings.editor.lineCol(editorStatus.line, editorStatus.col)}</span>
  <span>{strings.editor.words(words)}</span>
  {#if editorStatus.vimMode}
    <span class="vim">{editorStatus.vimMode.toUpperCase()}</span>
  {/if}
</footer>

<style>
  .statusbar {
    display: flex;
    flex: none;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-4);
    height: 30px;
    padding: 0 var(--space-4);
    border-top: 1px solid var(--line);
    color: var(--text-faint);
    font-size: var(--text-xs);
    font-variant-numeric: tabular-nums;
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
