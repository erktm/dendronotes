<script lang="ts">
  import EmptyState from './EmptyState.svelte'
  import Icon from './Icon.svelte'
  import Editor from './Editor.svelte'
  import ReadingView from './ReadingView.svelte'
  import StatusBar from './StatusBar.svelte'
  import ConflictView from './ConflictView.svelte'
  import type { NoteSession } from '../lib/state/note.svelte'
  import { settings } from '../lib/state/settings.svelte'
  import { displayName } from '../lib/paths'
  import { strings } from '../lib/strings'

  let { session }: { session: NoteSession } = $props()
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
{:else if session.conflict !== null}
  <ConflictView {session} />
{:else}
  <div class="note">
    {#if session.readOnly}
      <div class="banner">
        <Icon name="lock" size={17} />
        <p>{strings.note.readOnly}</p>
      </div>
    {/if}
    {#if settings.view === 'read'}
      <div class="scroll"><ReadingView {session} /></div>
    {:else}
      <div class="edit"><Editor {session} /></div>
      <StatusBar {session} />
    {/if}
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
    min-height: 0;
  }
  .edit {
    flex: 1;
    min-height: 0;
  }
  .scroll {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
  .banner {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-6);
    border-bottom: 1px solid var(--line);
    background: var(--bg-sunken);
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .banner p {
    flex: 1;
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
  @media (max-width: 760px) {
    .banner {
      padding: var(--space-3) var(--space-4);
    }
  }
</style>
