<script lang="ts">
  import EmptyState from './EmptyState.svelte'
  import Icon from './Icon.svelte'
  import type { NoteSession } from '../lib/state/note.svelte'
  import { displayName } from '../lib/paths'
  import { strings } from '../lib/strings'

  let { session }: { session: NoteSession } = $props()

  let editor: HTMLTextAreaElement | undefined = $state()

  $effect(() => {
    if (session.status === 'ready' && session.autofocus && editor) {
      editor.focus()
      session.autofocus = false
    }
  })
</script>

{#if session.status === 'loading'}
  <p class="loading">{strings.note.loading}</p>
{:else if session.status === 'missing'}
  <EmptyState title={strings.note.notFoundTitle} text={strings.note.notFoundText(displayName(session.path))} />
{:else if session.status === 'unsupported'}
  <EmptyState title={strings.note.unsupportedTitle} text={strings.note.unsupportedText} />
{:else if session.status === 'too-large'}
  <EmptyState title={strings.note.tooLargeTitle} text={strings.note.tooLargeText} />
{:else if session.status === 'error'}
  <EmptyState title={strings.note.errorTitle} text={strings.note.errorText}>
    <button type="button" class="button" onclick={() => void session.load()}>{strings.note.retry}</button>
  </EmptyState>
{:else}
  <div class="note">
    {#if session.conflict !== null}
      <div class="banner warn" role="alert">
        <Icon name="alert" size={18} />
        <p><strong>{strings.note.conflictTitle}</strong> {strings.note.conflictText}</p>
        <button type="button" class="button" onclick={() => void session.reloadFromDisk()}>{strings.note.useDisk}</button>
        <button type="button" class="button primary" onclick={() => void session.save(true)}>{strings.note.keepMine}</button>
      </div>
    {:else if session.readOnly}
      <div class="banner">
        <Icon name="lock" size={17} />
        <p>{strings.note.readOnly}</p>
      </div>
    {/if}
    <textarea
      class="editor"
      bind:this={editor}
      bind:value={session.content}
      oninput={() => session.scheduleSave()}
      readonly={session.readOnly}
      aria-label={strings.note.contentLabel(displayName(session.path))}
      placeholder={strings.note.placeholder}
      spellcheck="true"
    ></textarea>
  </div>
{/if}

<style>
  .loading {
    padding: var(--space-8);
    color: var(--text-faint);
    text-align: center;
  }
  .note {
    display: flex;
    flex-direction: column;
    height: 100%;
  }
  .banner {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    padding: var(--space-3) var(--space-6);
    border-bottom: 1px solid var(--line);
    background: var(--bg-sunken);
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .banner.warn {
    background: color-mix(in srgb, var(--warn) 12%, var(--bg));
    color: var(--text);
  }
  .banner.warn :global(.icon) {
    color: var(--warn);
  }
  .banner p {
    flex: 1;
    min-width: 16ch;
  }
  .editor {
    flex: 1;
    width: 100%;
    max-width: calc(var(--measure) + 2 * var(--space-8));
    margin: 0 auto;
    padding: var(--space-8);
    border: 0;
    outline: none;
    resize: none;
    background: transparent;
    color: var(--text);
    font-family: var(--font-mono);
    font-size: 0.9rem;
    line-height: 1.75;
    tab-size: 4;
  }
  .editor::placeholder {
    color: var(--text-faint);
  }
  @media (max-width: 760px) {
    .editor {
      padding: var(--space-6) var(--space-4);
    }
    .banner {
      padding: var(--space-3) var(--space-4);
    }
  }
  .button {
    height: 30px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    font-weight: 550;
    cursor: pointer;
  }
  .button:hover {
    background: var(--bg-hover);
  }
  .button.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--on-accent);
  }
</style>
