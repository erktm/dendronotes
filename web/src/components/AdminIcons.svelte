<script lang="ts">
  // The server's icon library: admins fill it for everyone (single pictures
  // or ZIP packs, whose folders become sections) and arrange its sections.
  import Icon from './Icon.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import * as api from '../lib/api/drawings'
  import type { IconInfo, IconLibrary, IconSection } from '../lib/api/drawings'
  import { app } from '../lib/state/app.svelte'
  import { errorMessage } from '../lib/errors'
  import { iconName, iconUpload } from '../lib/drawing/pictures'
  import { strings } from '../lib/strings'

  const t = strings.drawing.icons
  const a = strings.admin
  const lib = api.serverIcons

  let data = $state.raw<IconLibrary | null>(null)
  let target = $state(0)
  let busy = $state(false)
  let pictureInput = $state<HTMLInputElement>()
  let packInput = $state<HTMLInputElement>()
  let menu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null)

  const groups = $derived(
    data
      ? [
          ...data.sections.map((section) => ({ section, icons: data!.icons.filter((i) => i.sectionId === section.id) })),
          { section: null as IconSection | null, icons: data.icons.filter((i) => !i.sectionId) },
        ].filter((g) => g.section || g.icons.length)
      : [],
  )

  async function load() {
    try {
      data = await api.serverLibrary()
      if (target && !data.sections.some((s) => s.id === target)) target = 0
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  $effect(() => {
    void load()
  })

  async function run(fn: () => Promise<unknown>) {
    busy = true
    try {
      await fn()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
    await load()
  }

  function uploadPictures(files: FileList | null) {
    if (!files?.length) return
    void run(async () => {
      for (const f of files) {
        const picture = await iconUpload(f)
        if (!picture) {
          app.toast(`${f.name}: ${t.unreadable}`)
          continue
        }
        try {
          await lib.add(iconName(f), picture, target)
        } catch (err) {
          app.toast(`${f.name}: ${errorMessage(err)}`)
        }
      }
      if (pictureInput) pictureInput.value = ''
    })
  }

  function uploadPack(files: FileList | null) {
    const file = files?.[0]
    if (!file) return
    void run(async () => {
      const r = await lib.addPack(file)
      app.toast(t.packAdded(r.added, r.sections.length), 'info')
      if (r.skipped.length) app.toast(t.packSkipped(r.skipped.length, `${r.skipped[0]!.file} (${r.skipped[0]!.reason})`))
      if (packInput) packInput.value = ''
    })
  }

  async function newSection() {
    const name = await app.prompt({ title: t.sectionTitle, label: t.sectionName, value: '', confirmLabel: t.create })
    if (name?.trim()) void run(() => lib.addSection(name.trim()))
  }

  async function rename(sec: IconSection) {
    const name = await app.prompt({ title: t.renameTitle, label: t.sectionName, value: sec.name, confirmLabel: t.save })
    if (name?.trim() && name.trim() !== sec.name) void run(() => lib.renameSection(sec.id, name.trim()))
  }

  function shift(sec: IconSection, by: number) {
    const ids = data?.sections.map((s) => s.id) ?? []
    const i = ids.indexOf(sec.id)
    const j = i + by
    if (i < 0 || j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j]!, ids[i]!]
    void run(() => lib.orderSections(ids))
  }

  async function removeSection(sec: IconSection, count: number) {
    const ok = await app.ask({ title: t.deleteSectionTitle, message: t.deleteSectionMessage(sec.name, count), confirmLabel: strings.tree.delete, danger: true })
    if (ok) void run(() => lib.removeSection(sec.id))
  }

  async function removeIcon(icon: IconInfo) {
    const ok = await app.ask({ title: t.deleteTitle, message: t.deleteMessage(icon.name), confirmLabel: strings.tree.delete, danger: true })
    if (ok) void run(() => lib.remove(icon.id))
  }

  function iconMenu(e: MouseEvent, icon: IconInfo) {
    e.preventDefault()
    const items: MenuItem[] = []
    if (icon.sectionId) items.push({ label: t.outOfSection, icon: 'folder', action: () => void run(() => lib.move(icon.id, 0)) })
    for (const sec of data?.sections ?? []) {
      if (sec.id !== icon.sectionId) items.push({ label: t.moveTo(sec.name), icon: 'folder', action: () => void run(() => lib.move(icon.id, sec.id)) })
    }
    items.push({ label: t.delete(icon.name), icon: 'trash', danger: true, separator: items.length > 0, action: () => void removeIcon(icon) })
    menu = { x: e.clientX, y: e.clientY, items }
  }
</script>

<div class="admin-page">
  <div class="admin-card wide">
    <h3>{a.tabs.icons}</h3>
    <p class="admin-faint">{a.iconsText}</p>
    <div class="admin-actions">
      {#if data?.sections.length}
        <label class="to">
          {t.uploadTo}
          <select class="admin-field" bind:value={target}>
            <option value={0}>{t.noSection}</option>
            {#each data.sections as sec (sec.id)}<option value={sec.id}>{sec.name}</option>{/each}
          </select>
        </label>
      {/if}
      <button type="button" class="admin-btn" disabled={busy} onclick={() => pictureInput?.click()}><Icon name="upload" size={15} /> {t.upload}</button>
      <button type="button" class="admin-btn" disabled={busy} onclick={() => packInput?.click()}>{t.uploadPack}</button>
      <button type="button" class="admin-btn" disabled={busy} onclick={() => void newSection()}><Icon name="plus" size={15} /> {t.newSection}</button>
    </div>
    <input
      bind:this={pictureInput}
      type="file"
      accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
      multiple
      hidden
      onchange={(e) => uploadPictures(e.currentTarget.files)}
    />
    <input bind:this={packInput} type="file" accept=".zip,application/zip" hidden onchange={(e) => uploadPack(e.currentTarget.files)} />
  </div>

  {#if !data}
    <p class="admin-empty">{a.loading}</p>
  {:else if groups.length === 0}
    <p class="admin-empty">{a.iconsEmpty}</p>
  {:else}
    {#each groups as g, i (g.section?.id ?? 0)}
      <section class="admin-card wide group">
        <header>
          <h3>{g.section?.name ?? t.noSection} <span class="admin-faint">{g.icons.length}</span></h3>
          {#if g.section}
            {@const sec = g.section}
            <div class="tools">
              <button type="button" class="admin-btn small" disabled={busy} onclick={() => void rename(sec)}>{t.rename}</button>
              <button type="button" class="admin-btn small" disabled={busy || i === 0} aria-label={t.moveUp} title={t.moveUp} onclick={() => shift(sec, -1)}>↑</button>
              <button
                type="button"
                class="admin-btn small"
                disabled={busy || i === data.sections.length - 1}
                aria-label={t.moveDown}
                title={t.moveDown}
                onclick={() => shift(sec, 1)}>↓</button
              >
              <button type="button" class="admin-btn small danger" disabled={busy} onclick={() => void removeSection(sec, g.icons.length)}>{t.deleteSection}</button>
            </div>
          {/if}
        </header>
        {#if g.icons.length}
          <ul>
            {#each g.icons as icon (icon.id)}
              <li>
                <button type="button" class="tile" title={icon.name} aria-label={t.options(icon.name)} oncontextmenu={(e) => iconMenu(e, icon)} onclick={(e) => iconMenu(e, icon)}>
                  <img src={api.serverIconURL(icon.id)} alt="" loading="lazy" />
                  <span>{icon.name}</span>
                </button>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="admin-faint">{t.empty}</p>
        {/if}
      </section>
    {/each}
  {/if}
</div>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />
{/if}

<style>
  .to {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-sm);
  }
  .group header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-bottom: var(--space-3);
  }
  .group h3 {
    margin: 0;
  }
  .tools {
    display: flex;
    gap: 4px;
  }
  .small {
    height: 28px;
    padding: 0 10px;
    font-size: var(--text-xs);
  }
  .danger {
    color: var(--err);
  }
  ul {
    display: grid;
    margin: 0;
    padding: 0;
    grid-template-columns: repeat(auto-fill, minmax(88px, 1fr));
    gap: var(--space-2);
    list-style: none;
  }
  .tile {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
    width: 100%;
    padding: var(--space-2);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text-muted);
    font-size: var(--text-xs);
    cursor: pointer;
  }
  .tile:hover {
    border-color: var(--line-strong);
    background: var(--bg-hover);
  }
  .tile img {
    width: 40px;
    height: 40px;
    object-fit: contain;
  }
  .tile span {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
