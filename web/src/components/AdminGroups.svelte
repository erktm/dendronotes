<script lang="ts">
  import { closeOnBackdrop } from '../lib/actions'
  import Avatar from './Avatar.svelte'
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import * as admin from '../lib/api/admin'
  import type { Group, Person, User } from '../lib/api/soma'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  const t = strings.admin

  let groups = $state.raw<Group[] | null>(null)
  let users = $state.raw<User[]>([])
  let filter = $state('')

  async function load() {
    try {
      ;[groups, users] = await Promise.all([admin.groups(), admin.users()])
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }
  $effect(() => {
    void load()
  })

  const shown = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    return (groups ?? []).filter((g) => !q || g.name.toLowerCase().includes(q))
  })
  const label = (g: Group) => (g.everyone ? t.everyone : g.name)
  const name = (p: Person) => p.displayName || p.username

  async function create() {
    const value = await app.prompt({
      title: t.newGroupTitle,
      text: t.newGroupText,
      label: t.groupNameLabel,
      value: '',
      placeholder: t.groupNamePlaceholder,
      confirmLabel: t.createGroup,
    })
    if (!value) return
    try {
      const g = await admin.createGroup(value)
      app.toast(t.groupCreated(g.name), 'info')
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function rename(g: Group) {
    const value = await app.prompt({
      title: t.renameGroupTitle(g.name),
      label: t.groupNameLabel,
      value: g.name,
      confirmLabel: t.renameGroup,
    })
    if (!value || value === g.name) return
    try {
      const ng = await admin.renameGroup(g.id, value)
      app.toast(t.groupRenamed(ng.name), 'info')
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function remove(g: Group) {
    const ok = await app.ask({ title: t.deleteGroupTitle(g.name), message: t.deleteGroupText, confirmLabel: t.deleteGroup, danger: true })
    if (!ok) return
    try {
      await admin.deleteGroup(g.id)
      app.toast(t.groupDeleted(g.name), 'info')
      await load()
      await app.refreshSomas()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  // --- Members ---------------------------------------------------------------

  let dialog: HTMLDialogElement | undefined = $state()
  let current = $state.raw<admin.GroupMembers | null>(null)
  let adding = $state(0)
  let busy = $state(false)
  const candidates = $derived(users.filter((u) => !u.disabled && !current?.members.some((m) => m.id === u.id)))

  async function openMembers(g: Group) {
    try {
      current = await admin.groupMembers(g.id)
      adding = 0
      dialog?.showModal()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function change(action: () => Promise<admin.GroupMembers>) {
    busy = true
    try {
      current = await action()
      adding = 0
      await app.refreshSomas() // the admin may have gained or lost access themselves
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  function add(e: SubmitEvent) {
    e.preventDefault()
    const g = current?.group
    if (g && adding) void change(() => admin.addGroupMember(g.id, adding))
  }
</script>

<div class="admin-page">
  <div class="admin-toolbar">
    <input class="admin-filter" type="search" placeholder={t.groupFilter} aria-label={t.groupFilter} bind:value={filter} />
    <button type="button" class="admin-btn primary" onclick={create}><Icon name="plus" size={15} /> {t.newGroup}</button>
  </div>
  {#if groups === null}
    <p class="admin-empty">{t.loading}</p>
  {:else}
    <div class="admin-scroll">
      <table class="admin-table">
        <thead>
          <tr>
            <th>{t.group}</th>
            <th class="num">{t.members}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each shown as g (g.id)}
            <tr>
              <td>
                <div class="named">
                  <span class="icon" aria-hidden="true"><Icon name="group" size={16} /></span>
                  <div>
                    <strong>{label(g)}</strong>
                    {#if g.everyone}<div class="admin-faint">{t.everyoneDesc}</div>{/if}
                  </div>
                </div>
              </td>
              <td class="num">{g.members}</td>
              <td>
                {#if !g.everyone}
                  <div class="admin-actions" role="group" aria-label={t.actions(g.name)}>
                    <button type="button" class="admin-btn" onclick={() => openMembers(g)}>{t.manageMembers}</button>
                    <button type="button" class="admin-btn" onclick={() => rename(g)}>{t.renameGroup}</button>
                    <button type="button" class="admin-btn danger" onclick={() => remove(g)}>{t.deleteGroup}</button>
                  </div>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<dialog
  class="admin-dialog"
  bind:this={dialog}
  use:closeOnBackdrop
  aria-labelledby="group-members-title"
  onclose={() => {
    current = null
    void load()
  }}
>
  {#if current}
    {@const g = current.group}
    <form onsubmit={add}>
      <h2 id="group-members-title">{t.groupMembersTitle(g.name)}</h2>
      {#if current.members.length}
        <ul class="people">
          {#each current.members as m (m.id)}
            <li>
              <Avatar id={m.id} name={name(m)} avatar={m.avatar} size={26} />
              <span class="who"><strong>{name(m)}</strong><span class="admin-faint">{m.username}</span></span>
              <button
                type="button"
                class="remove"
                disabled={busy}
                aria-label={t.removeFromGroup(name(m))}
                title={t.removeFromGroup(name(m))}
                onclick={() => change(() => admin.removeGroupMember(g.id, m.id))}
              >
                <Icon name="close" size={15} />
              </button>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="admin-faint">{t.noGroupMembers}</p>
      {/if}
      <label for="group-add">{t.addToGroupLabel}</label>
      <div class="add">
        <select id="group-add" class="admin-field" bind:value={adding} disabled={busy || !candidates.length}>
          <option value={0}>–</option>
          {#each candidates as u (u.id)}
            <option value={u.id}>{u.displayName ? `${u.displayName} (${u.username})` : u.username}</option>
          {/each}
        </select>
        <button type="submit" class="admin-btn" disabled={busy || !adding}>{t.addToGroup}</button>
      </div>
      <div class="admin-actions">
        <button type="button" class="admin-btn primary" onclick={() => dialog?.close()}>{t.done}</button>
      </div>
    </form>
  {/if}
</dialog>

<style>
  .named {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .icon {
    display: grid;
    flex: none;
    place-items: center;
    width: 30px;
    height: 30px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-strong);
  }
  .people {
    margin: 10px 0 8px;
    padding: 0;
    list-style: none;
  }
  .people li {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 42px;
  }
  .people li + li {
    border-top: 1px solid var(--line);
  }
  .who {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
    font-size: var(--text-sm);
  }
  .remove {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .remove:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text);
  }
  .add {
    display: flex;
    gap: 8px;
  }
  .add select {
    flex: 1;
    min-width: 0;
  }
</style>
