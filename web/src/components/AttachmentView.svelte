<script lang="ts">
  import Icon from './Icon.svelte'
  import EmptyState from './EmptyState.svelte'
  import { app } from '../lib/state/app.svelte'
  import { fileKind, fileURL, formatSize } from '../lib/files'
  import { basename } from '../lib/paths'
  import { strings } from '../lib/strings'

  let { path }: { path: string } = $props()

  const entry = $derived(app.entries.find((e) => e.path === path && e.type === 'file'))
  const kind = $derived(fileKind(path))
  // The modification time makes a replaced file load again.
  const url = $derived(entry ? `${fileURL(path)}?v=${entry.mtime}` : fileURL(path))
  const name = $derived(basename(path))
  // English like the rest of the UI, whatever the browser's locale.
  const dateFormat = new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' })
  const modified = $derived(entry ? dateFormat.format(new Date(entry.mtime)) : '')
</script>

{#if app.treeLoaded && !entry}
  <EmptyState title={strings.note.notFoundTitle} text={strings.note.notFoundText(name)} />
{:else}
  <section class="viewer" aria-label={strings.files.viewerLabel(name)}>
    <header>
      <div class="meta">
        <h1>{name}</h1>
        {#if entry}
          <p>
            <span>{strings.files.size} {formatSize(entry.size ?? 0)}</span>
            <span>{strings.files.modified} {modified}</span>
          </p>
        {/if}
      </div>
      <div class="actions">
        <a class="btn" href={url} target="_blank" rel="noopener">
          <Icon name="external" size={16} />
          {strings.files.open}
        </a>
        <a class="btn" href={url} download={name}>
          <Icon name="download" size={16} />
          {strings.files.download}
        </a>
      </div>
    </header>

    <div class="stage">
      {#if kind === 'image'}
        <img src={url} alt={name} />
      {:else if kind === 'video'}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video src={url} controls preload="metadata"></video>
      {:else if kind === 'audio'}
        <audio src={url} controls preload="metadata"></audio>
      {:else}
        <p class="none">{strings.files.noPreview}</p>
      {/if}
    </div>
  </section>
{/if}

<style>
  .viewer {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  header {
    display: flex;
    flex-wrap: wrap;
    flex: none;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-6);
    border-bottom: 1px solid var(--line);
  }
  .meta {
    min-width: 0;
  }
  h1 {
    overflow: hidden;
    font-size: var(--text-lg);
    font-weight: 600;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  p {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-4);
    margin-top: 2px;
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
  .btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 32px;
    padding: 0 12px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg-raised);
    color: var(--text);
    font-size: var(--text-sm);
    font-weight: 550;
    text-decoration: none;
  }
  .btn:hover {
    border-color: var(--accent);
  }
  .stage {
    display: grid;
    flex: 1;
    min-height: 0;
    place-items: center;
    padding: var(--space-6);
    overflow: auto;
    background: var(--bg-sunken);
  }
  img,
  video {
    max-width: 100%;
    max-height: 100%;
    border-radius: 8px;
    box-shadow: var(--shadow);
  }
  img {
    background: repeating-conic-gradient(var(--bg-hover) 0 25%, var(--bg-raised) 0 50%) 0 0 / 16px 16px;
  }
  audio {
    width: min(100%, 520px);
  }
  .none {
    color: var(--text-faint);
  }
</style>
