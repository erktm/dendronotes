<script lang="ts">
  // Icons next to the drawing editor: the soma's library in its sections
  // (contributors upload pictures and ZIP packs and arrange sections; the
  // uploader or an editor deletes), the server's library (filled by admins)
  // and the built-in line icons, which take the drawing's colors. Every group
  // folds away; the browser remembers which.
  import { ICON_GROUPS } from '../lib/drawing/icons'
  import { onMount } from 'svelte'
  import Icon from './Icon.svelte'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import * as api from '../lib/api/drawings'
  import type { IconInfo, IconLibrary, IconSection } from '../lib/api/drawings'
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
  const t = strings.drawing.icons
  let query = $state('')
  const q = $derived(query.trim().toLowerCase())
  const matches = (label: string) => !q || label.toLowerCase().includes(q)

  const empty: IconLibrary = { icons: [], sections: [] }
  let own = $state.raw<IconLibrary>(empty)
  let shared = $state.raw<IconLibrary>(empty)
  let loaded = $state(false)
  let busy = $state(false)
  let target = $state(0)
  let pictureInput = $state<HTMLInputElement>()
  let packInput = $state<HTMLInputElement>()
  let menu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null)

  interface Group {
    key: string
    title: string
    section: IconSection | null
    server: boolean
    icons: IconInfo[]
  }

  /** The sections of a library and its icons without one, filtered. */
  function groupsOf(lib: IconLibrary, server: boolean, plain: string): Group[] {
    const out: Group[] = lib.sections.map((section) => ({
      key: `${server ? 's' : 'o'}${section.id}`,
      title: section.name,
      section,
      server,
      icons: lib.icons.filter((i) => i.sectionId === section.id && matches(i.name)),
    }))
    out.push({ key: server ? 's0' : 'o0', title: plain, section: null, server, icons: lib.icons.filter((i) => !i.sectionId && matches(i.name)) })
    return out
  }

  const ownGroups = $derived(groupsOf(own, false, b.uploaded).filter((g) => g.icons.length > 0 || (g.section && !q)))
  const serverGroups = $derived(groupsOf(shared, true, t.noSection).filter((g) => g.icons.length > 0))
  const builtin = $derived(
    ICON_GROUPS.map((g) => ({ id: g.id, icons: g.icons.filter((i) => matches(b.names[i.name] ?? i.name)) })).filter((g) => g.icons.length > 0),
  )
  const nothing = $derived(loaded && ownGroups.length === 0 && serverGroups.length === 0 && builtin.length === 0)

  // Folded groups, per browser.
  const FOLDED_KEY = 'dn.iconGroups.folded'
  let folded = $state(new Set<string>(readFolded()))
  function readFolded(): string[] {
    try {
      return JSON.parse(localStorage.getItem(FOLDED_KEY) ?? '[]') as string[]
    } catch {
      return []
    }
  }
  function toggle(key: string, open: boolean) {
    const next = new Set(folded)
    if (open) next.delete(key)
    else next.add(key)
    folded = next
    try {
      localStorage.setItem(FOLDED_KEY, JSON.stringify([...next]))
    } catch {
      // Only a convenience.
    }
  }

  async function load() {
    try {
      const r = await api.icons()
      own = { icons: r.icons, sections: r.sections }
      shared = r.server
      if (target && !own.sections.some((s) => s.id === target)) target = 0
    } catch (err) {
      app.toast(errorMessage(err))
    }
    loaded = true
  }

  onMount(() => void load())

  async function pick(icon: IconInfo, server: boolean) {
    try {
      const file = await pictureFile(server ? api.serverIconURL(icon.id) : api.iconURL(icon.id))
      if (file) onpick(file)
      else app.toast(t.unreadable)
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
        app.toast(`${f.name}: ${t.unreadable}`)
        continue
      }
      try {
        await api.somaIcons.add(iconName(f), picture, target)
      } catch (err) {
        app.toast(`${f.name}: ${errorMessage(err)}`)
      }
    }
    busy = false
    if (pictureInput) pictureInput.value = ''
    await load()
  }

  async function uploadPack(files: FileList | null) {
    const file = files?.[0]
    if (!file || !app.may('create')) return
    busy = true
    try {
      const r = await api.somaIcons.addPack(file)
      app.toast(t.packAdded(r.added, r.sections.length), 'info')
      if (r.skipped.length) app.toast(t.packSkipped(r.skipped.length, `${r.skipped[0]!.file} (${r.skipped[0]!.reason})`))
    } catch (err) {
      app.toast(errorMessage(err))
    }
    busy = false
    if (packInput) packInput.value = ''
    await load()
  }

  async function newSection() {
    if (!app.may('create')) return
    const name = await app.prompt({ title: t.sectionTitle, label: t.sectionName, value: '', confirmLabel: t.create })
    if (!name?.trim()) return
    try {
      const sec = await api.somaIcons.addSection(name.trim())
      target = sec.id
    } catch (err) {
      app.toast(errorMessage(err))
    }
    await load()
  }

  async function renameSection(sec: IconSection) {
    const name = await app.prompt({ title: t.renameTitle, label: t.sectionName, value: sec.name, confirmLabel: t.save })
    if (!name?.trim() || name.trim() === sec.name) return
    try {
      await api.somaIcons.renameSection(sec.id, name.trim())
    } catch (err) {
      app.toast(errorMessage(err))
    }
    await load()
  }

  async function shiftSection(sec: IconSection, by: number) {
    const ids = own.sections.map((s) => s.id)
    const i = ids.indexOf(sec.id)
    const j = i + by
    if (i < 0 || j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j]!, ids[i]!]
    try {
      await api.somaIcons.orderSections(ids)
    } catch (err) {
      app.toast(errorMessage(err))
    }
    await load()
  }

  async function deleteSection(sec: IconSection) {
    const n = own.icons.filter((i) => i.sectionId === sec.id).length
    const ok = await app.ask({ title: t.deleteSectionTitle, message: t.deleteSectionMessage(sec.name, n), confirmLabel: strings.tree.delete, danger: true })
    if (!ok) return
    try {
      await api.somaIcons.removeSection(sec.id)
    } catch (err) {
      app.toast(errorMessage(err))
    }
    await load()
  }

  async function remove(icon: IconInfo) {
    const ok = await app.ask({ title: t.deleteTitle, message: t.deleteMessage(icon.name), confirmLabel: strings.tree.delete, danger: true })
    if (!ok) return
    try {
      await api.somaIcons.remove(icon.id)
    } catch (err) {
      app.toast(errorMessage(err))
    }
    await load()
  }

  async function moveIcon(icon: IconInfo, section: number) {
    try {
      await api.somaIcons.move(icon.id, section)
    } catch (err) {
      app.toast(errorMessage(err))
    }
    await load()
  }

  const mayDelete = (icon: IconInfo) => icon.createdBy === app.user?.id || app.can.organize

  function where(e: MouseEvent): { x: number; y: number } {
    if (e.clientX === 0 && e.clientY === 0 && e.currentTarget instanceof HTMLElement) {
      const r = e.currentTarget.getBoundingClientRect()
      return { x: r.left, y: r.bottom }
    }
    return { x: e.clientX, y: e.clientY }
  }

  function sectionMenu(e: MouseEvent, sec: IconSection) {
    e.preventDefault()
    e.stopPropagation()
    const i = own.sections.findIndex((s) => s.id === sec.id)
    const items: MenuItem[] = [{ label: t.rename, icon: 'pencil', action: () => void renameSection(sec) }]
    if (i > 0) items.push({ label: t.moveUp, icon: 'chevron', action: () => void shiftSection(sec, -1) })
    if (i < own.sections.length - 1) items.push({ label: t.moveDown, icon: 'chevron-down', action: () => void shiftSection(sec, 1) })
    if (app.can.organize) items.push({ label: t.deleteSection, icon: 'trash', danger: true, action: () => void deleteSection(sec) })
    menu = { ...where(e), items }
  }

  function iconMenu(e: MouseEvent, icon: IconInfo) {
    if (!app.can.create) return
    e.preventDefault()
    const items: MenuItem[] = []
    if (icon.sectionId) items.push({ label: t.outOfSection, icon: 'folder', action: () => void moveIcon(icon, 0) })
    for (const sec of own.sections) {
      if (sec.id !== icon.sectionId) items.push({ label: t.moveTo(sec.name), icon: 'folder', action: () => void moveIcon(icon, sec.id) })
    }
    if (mayDelete(icon)) items.push({ label: t.delete(icon.name), icon: 'trash', danger: true, separator: items.length > 0, action: () => void remove(icon) })
    if (items.length) menu = { ...where(e), items }
  }
</script>

{#snippet group(g: Group)}
  <details class="group" open={!folded.has(g.key) || !!q} ontoggle={(e) => !q && toggle(g.key, e.currentTarget.open)}>
    <summary aria-label={t.collapse(g.title)}>
      <span class="title">{g.title}</span>
      <span class="count">{g.icons.length}</span>
      {#if g.section && !g.server && app.can.create}
        <button type="button" class="icon-btn small" title={t.options(g.title)} aria-label={t.options(g.title)} onclick={(e) => sectionMenu(e, g.section!)}>
          <Icon name="more" size={14} />
        </button>
      {/if}
    </summary>
    {#if g.icons.length}
      <ul>
        {#each g.icons as icon (icon.id)}
          <li>
            <button
              type="button"
              class="pick"
              title={icon.name}
              aria-label={t.insert(icon.name)}
              onclick={() => void pick(icon, g.server)}
              oncontextmenu={(e) => !g.server && iconMenu(e, icon)}
            >
              <img src={g.server ? api.serverIconURL(icon.id) : api.iconURL(icon.id)} alt="" loading="lazy" />
            </button>
            {#if !g.server && mayDelete(icon)}
              <button type="button" class="remove" title={t.delete(icon.name)} aria-label={t.delete(icon.name)} onclick={() => void remove(icon)}>
                <Icon name="close" size={12} />
              </button>
            {/if}
          </li>
        {/each}
      </ul>
    {:else}
      <p class="empty">{t.empty}</p>
    {/if}
  </details>
{/snippet}

<aside class="library" aria-label={t.title}>
  <header>
    <h2>{t.title}</h2>
    <button type="button" class="icon-btn" title={t.close} aria-label={t.close} onclick={onclose}>
      <Icon name="close" size={16} />
    </button>
  </header>
  <p class="hint">{t.hint}</p>
  {#if app.can.create}
    <div class="actions">
      {#if own.sections.length}
        <label class="to">
          <span>{t.uploadTo}</span>
          <select bind:value={target}>
            <option value={0}>{t.noSection}</option>
            {#each own.sections as sec (sec.id)}<option value={sec.id}>{sec.name}</option>{/each}
          </select>
        </label>
      {/if}
      <button type="button" class="upload" disabled={busy} onclick={() => pictureInput?.click()}>
        <Icon name="upload" size={16} />
        {busy ? t.uploading : t.upload}
      </button>
      <div class="row">
        <button type="button" class="upload small" disabled={busy} title={t.uploadPack} onclick={() => packInput?.click()}>{t.uploadPackShort}</button>
        <button type="button" class="upload small" disabled={busy} onclick={() => void newSection()}>{t.newSection}</button>
      </div>
    </div>
    <input
      bind:this={pictureInput}
      type="file"
      accept="image/png,image/jpeg,image/webp,image/gif,image/svg+xml"
      multiple
      hidden
      onchange={(e) => void upload(e.currentTarget.files)}
    />
    <input bind:this={packInput} type="file" accept=".zip,application/zip" hidden onchange={(e) => void uploadPack(e.currentTarget.files)} />
  {/if}
  <input class="filter" type="search" placeholder={b.filter} aria-label={b.filter} bind:value={query} />

  {#each ownGroups as g (g.key)}{@render group(g)}{/each}
  {#if serverGroups.length}<h3 class="source">{t.server}</h3>{/if}
  {#each serverGroups as g (g.key)}{@render group(g)}{/each}
  {#if builtin.length}<h3 class="source">{b.title}</h3>{/if}
  {#each builtin as g (g.id)}
    <details class="group" open={!folded.has(`b${g.id}`) || !!q} ontoggle={(e) => !q && toggle(`b${g.id}`, e.currentTarget.open)}>
      <summary aria-label={t.collapse(b.groups[g.id] ?? g.id)}>
        <span class="title">{b.groups[g.id]}</span>
        <span class="count">{g.icons.length}</span>
      </summary>
      <ul class="builtin">
        {#each g.icons as icon (icon.name)}
          <li>
            <button
              type="button"
              class="pick"
              title={b.names[icon.name]}
              aria-label={t.insert(b.names[icon.name] ?? icon.name)}
              onclick={() => onbuiltin(icon.name)}
            >
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d={icon.d} /></svg>
            </button>
          </li>
        {/each}
      </ul>
    </details>
  {/each}
  {#if q && nothing}
    <p class="empty">{b.none}</p>
  {/if}
</aside>

{#if menu}
  <ContextMenu x={menu.x} y={menu.y} items={menu.items} onclose={() => (menu = null)} />
{/if}

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
  .icon-btn.small {
    width: 22px;
    height: 22px;
  }
  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .actions {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .row {
    display: flex;
    gap: var(--space-2);
  }
  .to {
    display: flex;
    flex-direction: column;
    gap: 2px;
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  .to select {
    height: 28px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font-size: var(--text-sm);
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
  .upload.small {
    flex: 1;
    height: 28px;
    padding: 0 var(--space-1);
    font-size: var(--text-xs);
  }
  .upload:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .source {
    margin-top: var(--space-2);
    padding-top: var(--space-2);
    border-top: 1px solid var(--line);
    color: var(--text);
    font-size: var(--text-xs);
    font-weight: 650;
  }
  .group summary {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    min-height: 24px;
    color: var(--text-muted);
    font-size: var(--text-xs);
    font-weight: 600;
    cursor: pointer;
    list-style: none;
  }
  .group summary::-webkit-details-marker {
    display: none;
  }
  .group summary::before {
    content: '▸';
    font-size: 10px;
    transition: transform 0.12s ease;
  }
  .group[open] summary::before {
    transform: rotate(90deg);
  }
  .group summary .title {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .group summary .count {
    color: var(--text-faint);
    font-weight: 500;
  }
  .group > ul,
  .group > .empty {
    margin-top: var(--space-2);
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
