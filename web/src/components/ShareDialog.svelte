<script lang="ts">
  import { closeOnBackdrop } from '../lib/actions'
  import Icon from './Icon.svelte'
  import Avatar from './Avatar.svelte'
  import { app } from '../lib/state/app.svelte'
  import * as api from '../lib/api/soma'
  import type { Group, GroupGrant, Member, Person, ShareRole } from '../lib/api/soma'
  import { SHARE_ROLES } from '../lib/roles'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  const t = strings.share
  /** Most to least, as offered in the menus. */
  const roles = [...SHARE_ROLES].reverse()

  let dialog: HTMLDialogElement | undefined = $state()
  let members = $state.raw<Member[]>([])
  let groups = $state.raw<GroupGrant[]>([])
  let query = $state('')
  let found = $state.raw<Person[]>([])
  let foundGroups = $state.raw<Group[]>([])
  let searched = $state(false)
  let newRole = $state<ShareRole>('editor')
  let busy = $state(false)

  const soma = $derived(app.soma)
  const owner = $derived(soma?.role === 'owner')
  const name = (p: Person) => p.displayName || p.username
  const groupName = (g: Group) => (g.everyone ? t.everyone : g.name)

  $effect(() => {
    if (!dialog) return
    if (app.shareOpen && !dialog.open) {
      query = ''
      found = []
      foundGroups = []
      members = []
      groups = []
      dialog.showModal()
      void load()
    } else if (!app.shareOpen && dialog.open) dialog.close()
  })

  // Closing when the soma goes away (deleted, access taken).
  $effect(() => {
    if (app.shareOpen && !soma) app.shareOpen = false
  })

  async function load() {
    if (!soma) return
    try {
      const m = await api.members(soma.id)
      members = m.members
      groups = m.groups
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  // Search as you type (from two characters), skipping who has access already.
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  $effect(() => {
    const q = query.trim()
    clearTimeout(timer)
    controller?.abort()
    if ([...q].length < 2) {
      found = []
      foundGroups = []
      searched = false
      return
    }
    timer = setTimeout(async () => {
      controller = new AbortController()
      try {
        const res = await api.searchPeople(q, controller.signal)
        found = res.users.filter((p) => !members.some((m) => m.id === p.id))
        foundGroups = res.groups.filter((g) => !groups.some((x) => x.id === g.id))
        searched = true
      } catch (err) {
        if (!(err instanceof DOMException && err.name === 'AbortError')) app.toast(errorMessage(err))
      }
    }, 200)
  })

  async function run(action: () => Promise<api.Members>, done: string) {
    busy = true
    try {
      const m = await action()
      members = m.members
      groups = m.groups
      app.toast(done, 'info')
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  async function add(p: Person) {
    if (!soma) return
    await run(() => api.setMember(soma.id, p.id, newRole), t.added(name(p)))
    query = ''
  }

  async function addGroup(g: Group) {
    if (!soma) return
    await run(() => api.setSomaGroup(soma.id, g.id, newRole), t.added(groupName(g)))
    query = ''
  }

  function changeRole(m: Member, role: ShareRole) {
    if (soma && role !== m.role) void run(() => api.setMember(soma.id, m.id, role), t.roleOf(name(m)) + ': ' + t.roles[role])
  }

  function changeGroupRole(g: GroupGrant, role: ShareRole) {
    if (soma && role !== g.role) void run(() => api.setSomaGroup(soma.id, g.id, role), t.roleOf(groupName(g)) + ': ' + t.roles[role])
  }

  function removeGroup(g: GroupGrant) {
    if (soma) void run(() => api.removeSomaGroup(soma.id, g.id), t.removed(groupName(g)))
  }

  function remove(m: Member) {
    if (soma) void run(() => api.removeMember(soma.id, m.id), t.removed(name(m)))
  }

  async function makeOwner(m: Member) {
    if (!soma) return
    const ok = await app.ask({
      title: t.transferTitle,
      message: t.transferMessage(name(m), soma.name),
      confirmLabel: t.transferButton,
    })
    if (!ok) return
    busy = true
    try {
      await api.transferSoma(soma.id, m.id)
      app.toast(t.transferred(name(m)), 'info')
      await app.refreshSomas()
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }
</script>

<dialog bind:this={dialog} use:closeOnBackdrop aria-labelledby="share-title" onclose={() => (app.shareOpen = false)}>
  <header>
    <h2 id="share-title">{soma ? (owner ? t.title(soma.name) : t.membersTitle(soma.name)) : ''}</h2>
    <button type="button" class="icon-btn" aria-label={t.close} onclick={() => (app.shareOpen = false)}>
      <Icon name="close" size={18} />
    </button>
  </header>

  {#if soma}
    {#if owner}
      <section class="section">
        <p class="intro">{t.intro}</p>
        <label for="share-search">{t.addLabel}</label>
        <div class="add">
          <input
            id="share-search"
            type="search"
            placeholder={t.searchPlaceholder}
            autocomplete="off"
            spellcheck="false"
            bind:value={query}
            aria-describedby="share-hint"
          />
          <select aria-label={t.newRole} bind:value={newRole} aria-describedby="share-role-hint">
            {#each roles as r (r)}
              <option value={r}>{t.roles[r]}</option>
            {/each}
          </select>
        </div>
        <p class="hint" id="share-role-hint">{t.roles[newRole]}: {t.roleHints[newRole]}</p>
        {#if found.length || foundGroups.length}
          <ul class="found">
            {#each foundGroups as g (g.id)}
              <li>
                <button type="button" disabled={busy} onclick={() => void addGroup(g)} aria-label={t.add(groupName(g))}>
                  <span class="avatar" aria-hidden="true"><Icon name="group" size={15} /></span>
                  <span class="who">
                    <strong>{groupName(g)}</strong>
                    <span class="faint">{g.everyone ? t.everyoneHint : t.groupSize(g.members)}</span>
                  </span>
                  <Icon name="plus" size={15} />
                </button>
              </li>
            {/each}
            {#each found as p (p.id)}
              <li>
                <button type="button" disabled={busy} onclick={() => void add(p)} aria-label={t.add(name(p))}>
                  <span class="avatar" aria-hidden="true"><Icon name="user" size={14} /></span>
                  <span class="who"><strong>{name(p)}</strong><span class="faint">{p.username}</span></span>
                  <Icon name="plus" size={15} />
                </button>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="hint" id="share-hint">{searched && query.trim() ? t.noMatches : t.searchHint}</p>
        {/if}
      </section>
    {/if}

    <section class="section">
      <h3>{t.people}</h3>
      <ul class="members">
        {#each members as m (m.id)}
          {@const me = m.id === app.user?.id}
          <li>
            <Avatar id={m.id} name={name(m)} avatar={m.avatar} size={28} />
            <span class="who">
              <strong>{name(m)}{#if me}<span class="faint"> ({t.you})</span>{/if}</strong>
              <span class="faint">{m.username}</span>
            </span>
            {#if m.role === 'owner'}
              <span class="role">{t.owner}</span>
            {:else if owner}
              <select
                aria-label={t.roleOf(name(m))}
                value={m.role}
                disabled={busy}
                onchange={(e) => changeRole(m, e.currentTarget.value as ShareRole)}
              >
                {#each roles as r (r)}
                  <option value={r}>{t.roles[r]}</option>
                {/each}
              </select>
              <button type="button" class="text-btn" disabled={busy} title={t.makeOwnerOf(name(m))} aria-label={t.makeOwnerOf(name(m))} onclick={() => void makeOwner(m)}>
                {t.makeOwner}
              </button>
              <button type="button" class="icon-btn" disabled={busy} aria-label={t.remove(name(m))} title={t.remove(name(m))} onclick={() => remove(m)}>
                <Icon name="close" size={15} />
              </button>
            {:else}
              <span class="role">{t.roles[m.role]}</span>
            {/if}
          </li>
        {/each}
      </ul>
      {#if groups.length}
        <h3 class="groups-title">{t.groups}</h3>
        <ul class="members">
          {#each groups as g (g.id)}
            <li>
              <span class="avatar" aria-hidden="true"><Icon name="group" size={15} /></span>
              <span class="who">
                <strong>{groupName(g)}</strong>
                <span class="faint">{g.everyone ? t.everyoneHint : t.groupSize(g.members)}</span>
              </span>
              {#if owner}
                <select
                  aria-label={t.roleOf(groupName(g))}
                  value={g.role}
                  disabled={busy}
                  onchange={(e) => changeGroupRole(g, e.currentTarget.value as ShareRole)}
                >
                  {#each roles as r (r)}
                    <option value={r}>{t.roles[r]}</option>
                  {/each}
                </select>
                <button type="button" class="icon-btn" disabled={busy} aria-label={t.remove(groupName(g))} title={t.remove(groupName(g))} onclick={() => removeGroup(g)}>
                  <Icon name="close" size={15} />
                </button>
              {:else}
                <span class="role">{t.roles[g.role]}</span>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
      {#if !owner && soma.direct}
        <div class="actions">
          <button type="button" class="danger" onclick={() => void app.leaveSoma(soma.id)}>{t.leave}</button>
        </div>
      {/if}
    </section>
  {/if}
</dialog>

<style>
  dialog {
    width: min(92vw, 520px);
    max-height: 90vh;
    padding: 0;
    overflow: auto;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow), 0 24px 64px rgb(0 0 0 / 0.2);
  }
  dialog::backdrop {
    background: var(--scrim);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 16px 16px 16px 20px;
    border-bottom: 1px solid var(--line);
  }
  h2 {
    min-width: 0;
    overflow: hidden;
    font-size: var(--text-lg);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  h3 {
    margin: 0 0 8px;
    font-size: 11.5px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .section {
    display: flex;
    flex-direction: column;
    padding: 16px 20px 18px;
  }
  .section + .section {
    border-top: 1px solid var(--line);
  }
  .intro,
  .hint {
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .intro {
    margin-bottom: 12px;
  }
  .hint {
    margin-top: 8px;
    font-size: var(--text-xs);
  }
  label {
    margin-bottom: 6px;
    font-size: var(--text-sm);
    font-weight: 550;
  }
  .add {
    display: flex;
    gap: 8px;
  }
  input,
  select {
    height: 36px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
  }
  input {
    flex: 1;
    min-width: 0;
  }
  input:focus-visible,
  select:focus-visible,
  button:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .found {
    margin-top: 8px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
  }
  .found li + li {
    border-top: 1px solid var(--line);
  }
  .found button {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 8px 10px;
    border: 0;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .found button:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .members li {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 44px;
  }
  .members li + li {
    border-top: 1px solid var(--line);
  }
  .avatar {
    display: grid;
    flex: none;
    place-items: center;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-strong);
  }
  .who {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
    font-size: var(--text-sm);
  }
  .who strong {
    overflow: hidden;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .faint {
    color: var(--text-faint);
    font-size: var(--text-xs);
    font-weight: 400;
  }
  .role {
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .groups-title {
    margin-top: 18px;
  }
  .members select {
    height: 30px;
  }
  .icon-btn {
    display: grid;
    flex: none;
    place-items: center;
    width: 30px;
    height: 30px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .icon-btn:hover:not(:disabled),
  .text-btn:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text);
  }
  .text-btn {
    height: 30px;
    padding: 0 8px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    font: inherit;
    font-size: var(--text-xs);
    white-space: nowrap;
    cursor: pointer;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 14px;
  }
  .danger {
    height: 34px;
    padding: 0 var(--space-4);
    border: 1px solid var(--err);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--err);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }
  button:disabled {
    cursor: default;
    opacity: 0.5;
  }
  @media (max-width: 520px) {
    .text-btn {
      display: none;
    }
  }
</style>
