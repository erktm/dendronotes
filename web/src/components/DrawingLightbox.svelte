<script lang="ts">
  // A drawing shown large over the note: the editor without changing
  // anything (move and zoom only). Writers can switch to editing.
  import DrawingEditor from './DrawingEditor.svelte'
  import { closeOnBackdrop } from '../lib/actions'
  import { app } from '../lib/state/app.svelte'
  import { drawingTitle } from '../lib/drawing/svg'
  import { strings } from '../lib/strings'

  let { path, onclose }: { path: string; onclose: () => void } = $props()

  let dialog = $state<HTMLDialogElement>()

  $effect(() => {
    if (dialog && !dialog.open) dialog.showModal()
  })

  function edit() {
    dialog?.close()
    void app.navigate(path)
  }
</script>

<dialog bind:this={dialog} class="drawing-lightbox" use:closeOnBackdrop aria-label={strings.drawing.label(drawingTitle(path))} {onclose}>
  <DrawingEditor {path} readonly doneLabel={strings.drawing.closeViewer} ondone={() => dialog?.close()} onedit={app.can.write ? edit : undefined} />
</dialog>

<style>
  .drawing-lightbox {
    width: min(1200px, 96vw);
    max-width: none;
    height: min(900px, 92vh);
    max-height: none;
    padding: 0;
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text);
    box-shadow: var(--shadow);
  }
  .drawing-lightbox::backdrop {
    background: var(--scrim);
  }
  @media (max-width: 760px) {
    .drawing-lightbox {
      width: 100vw;
      height: 100dvh;
      border: none;
      border-radius: 0;
    }
  }
</style>
