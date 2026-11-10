<script lang="ts">
  // Icons next to the drawing editor: the soma's uploaded pictures (upload
  // new ones as contributor, delete your own, editors any) and the built-in
  // line icons, which take the drawing's colors.
  import { ICON_GROUPS } from '../lib/drawing/icons'
  import { onMount } from 'svelte'
  import Icon from './Icon.svelte'
  import * as api from '../lib/api/drawings'
  import type { IconInfo } from '../lib/api/drawings'
  import { app } from '../lib/state/app.svelte'
  import { errorMessage } from '../lib/errors'
  import { iconName, iconUpload, pictureFile } from '../lib/drawing/pictures'
  import type { FileData } from '../lib/drawing/scene'
  import { strings } from '../lib/strings'

  let {
    onpick,
    onbuiltin,
    onclose,
  }: { onpick: (file: FileData) => void; onbuiltin: (name: string) => void; onclose: () => void } = $props()

  const b = strings.drawing.builtin
  let query = $state('')
  const q = $derived(query.trim().toLowerCase())
  const matches = (label: string) => !q || label.toLowerCase().includes(q)
  const groups = $derived(
    ICON_GROUPS.map((g) => ({ id: g.id, icons: g.icons.filter((i) => matches(b.names[i.name] ?? i.name)) })).filter((g) => g.icons.length > 0),
  )

  let list = $state.raw<IconInfo[]>([])
  const uploaded = $derived(list.filter((i) => matches(i.name)))
  let loaded = $state(false)
  let busy = $state(false)
  let input = $state<HTMLInputElement>()

  async function load() {
    try {
      list = await api.icons()
    } catch (err) {
      app.toast(errorMessage(err))
    }
    loaded = true
  }

  onMount(() => void load())

  async function pick(icon: IconInfo) {
    try {
      const file = await pictureFile(api.iconURL(icon.id))
      if (file) onpick(file)
      else app.toast(strings.drawing.icons.unreadable)
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function upload(files: FileList | null) {
    if (!files?.length || !app.may('create')) return
    busy = true
    for (const f of files) {
      const picture = await iconUpload(f)
      if (!picture) {
        app.toast(`${f.name}: ${strings.drawing.icons.unreadable}`)
        continue
      }
      try {
        await api.addIcon(iconName(f), picture)
      } catch (err) {
        app.toast(`${f.name}: ${errorMessage(err)}`)
      }
    }
    busy = false
    if (input) input.value = ''
    await load()
  }

  async function remove(icon: IconInfo) {
    const ok = await app.ask({
      title: strings.drawing.icons.deleteTitle,
      message: strings.drawing.icons.deleteMessage(icon.name),
      confirmLabel: strings.tree.delete,
      danger: true,
    })
    if (!ok) return
    try {
      await api.deleteIcon(icon.id)
    } catch (err) {
      app.toast(errorMessage(err))
    }
    await load()
  }

  const mayDelete = (icon: IconInfo) => icon.createdBy === app.user?.id || app.can.organize
</script>

<aside class="library" aria-label={strings.drawing.icons.title}>
  <header>
    <h2>{strings.drawing.icons.title}</h2>
    <button type="button" class="icon-btn" title={strings.drawing.icons.close} aria-label={strings.drawing.icons.close} onclick={onclose}>
      <Icon name="close" size={16} />
    </button>
  </header>
  <p class="hint">{strings.drawing.icons.hint}</p>
  {#if app.can.create}
    <button type="button" class="upload" disabled={busy} onclick={() => input?.click()}>
      <Icon name="upload" size={16} />
      {busy ? strings.drawing.icons.uploading : strings.drawing.icons.upload}
    </button>
    <input
      bind:this={input}
      type="file"
      accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
      multiple
      hidden
      onchange={(e) => void upload(e.currentTarget.files)}
    />
  {/if}
  <input class="filter" type="search" placeholder={b.filter} aria-label={b.filter} bind:value={query} />

  {#if uploaded.length > 0}
    <h3>{b.uploaded}</h3>
  {:else if loaded && list.length === 0 && !q}
    <p class="empty">{strings.drawing.icons.empty}</p>
  {/if}
  <ul>
    {#each uploaded as icon (icon.id)}
      <li>
        <button type="button" class="pick" title={icon.name} aria-label={strings.drawing.icons.insert(icon.name)} onclick={() => void pick(icon)}>
          <img src={api.iconURL(icon.id)} alt="" loading="lazy" />
        </button>
        {#if mayDelete(icon)}
          <button type="button" class="remove" title={strings.drawing.icons.delete(icon.name)} aria-label={strings.drawing.icons.delete(icon.name)} onclick={() => void remove(icon)}>
            <Icon name="close" size={12} />
          </button>
        {/if}
      </li>
    {/each}
  </ul>

  {#each groups as g (g.id)}
    <h3>{b.groups[g.id]}</h3>
    <ul class="builtin">
      {#each g.icons as icon (icon.name)}
        <li>
          <button
            type="button"
            class="pick"
            title={b.names[icon.name]}
            aria-label={strings.drawing.icons.insert(b.names[icon.name] ?? icon.name)}
            onclick={() => onbuiltin(icon.name)}
          >
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d={icon.d} /></svg>
          </button>
        </li>
      {/each}
    </ul>
  {/each}
  {#if q && groups.length === 0 && uploaded.length === 0}
    <p class="empty">{b.none}</p>
  {/if}
</aside>

<style>
  .library {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    width: 240px;
    padding: var(--space-3);
    overflow: auto;
    border-left: 1px solid var(--line);
    background: var(--bg-raised);
  }
  /* Stays in view while the icons scroll. */
  header {
    position: sticky;
    z-index: 1;
    top: calc(var(--space-3) * -1);
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin: calc(var(--space-3) * -1) calc(var(--space-3) * -1) 0;
    padding: var(--space-3);
    background: var(--bg-raised);
  }
  h2 {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .hint,
  .empty {
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  .icon-btn {
    display: grid;
    padding: 0;
    border: none;
    background: none;
    place-items: center;
    width: 28px;
    height: 28px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
  }
  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .upload {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    height: 32px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    font-size: var(--text-sm);
    font-weight: 550;
  }
  .upload:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  ul {
    display: grid;
    margin: 0;
    padding: 0;
    grid-template-columns: repeat(auto-fill, minmax(56px, 1fr));
    gap: var(--space-2);
    list-style: none;
  }
  li {
    position: relative;
  }
  .pick {
    display: grid;
    place-items: center;
    width: 100%;
    aspect-ratio: 1;
    padding: var(--space-1);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg);
  }
  .pick:hover {
    border-color: var(--line-strong);
    background: var(--bg-hover);
  }
  .pick img {
    max-width: 100%;
    max-height: 100%;
  }
  h3 {
    margin-top: var(--space-1);
    color: var(--text-muted);
    font-size: var(--text-xs);
    font-weight: 600;
  }
  .filter {
    height: 30px;
    padding: 0 var(--space-2);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font-size: var(--text-sm);
  }
  .builtin {
    grid-template-columns: repeat(auto-fill, minmax(38px, 1fr));
    gap: var(--space-1);
  }
  .builtin .pick {
    color: var(--text);
  }
  .builtin svg {
    width: 22px;
    height: 22px;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.7;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  .remove {
    position: absolute;
    top: -6px;
    right: -6px;
    display: none;
    place-items: center;
    width: 20px;
    height: 20px;
    border: 1px solid var(--line-strong);
    border-radius: 50%;
    background: var(--bg-raised);
    color: var(--text-muted);
  }
  li:hover .remove,
  .remove:focus-visible {
    display: grid;
  }
  @media (hover: none) {
    .remove {
      display: grid;
    }
  }
  @media (max-width: 760px) {
    .library {
      width: auto;
      max-height: 45vh;
      border-top: 1px solid var(--line);
      border-left: none;
    }
  }
</style>
