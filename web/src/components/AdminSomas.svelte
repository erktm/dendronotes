<script lang="ts">
  import { closeOnBackdrop } from '../lib/actions'
  import { app } from '../lib/state/app.svelte'
  import * as admin from '../lib/api/admin'
  import type { AdminSoma } from '../lib/api/admin'
  import type { Group, ShareRole, User } from '../lib/api/soma'
  import { SHARE_ROLES } from '../lib/roles'
  import Icon from './Icon.svelte'
  import { formatSize } from '../lib/files'
  import { ago } from '../lib/time'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  const t = strings.admin

  let somas = $state.raw<AdminSoma[] | null>(null)
  let deleted = $state.raw<admin.DeletedSoma[]>([])
  let users = $state.raw<User[]>([])
  let groups = $state.raw<Group[]>([])
  let filter = $state('')

  async function load() {
    try {
      ;[somas, users, groups, deleted] = await Promise.all([admin.somas(), admin.users(), admin.groups(), admin.deletedSomas()])
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }
  $effect(() => {
    void load()
  })

  const shown = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    return (somas ?? []).filter(
      (s) => !q || s.name.toLowerCase().includes(q) || s.owner.username.toLowerCase().includes(q) || s.folder.toLowerCase().includes(q),
    )
  })

  let dialog: HTMLDialogElement | undefined = $state()
  let target = $state.raw<AdminSoma | null>(null)
  let owner = $state(0)
  let busy = $state(false)

  function openTransfer(s: AdminSoma) {
    target = s
    owner = users.find((u) => u.id !== s.ownerId)?.id ?? s.ownerId
    dialog?.showModal()
  }

  async function transfer(e: SubmitEvent) {
    e.preventDefault()
    if (!target) return
    busy = true
    try {
      await admin.transferSoma(target.id, owner)
      const to = users.find((u) => u.id === owner)
      app.toast(t.transferred(target.name, to?.displayName || to?.username || ''), 'info')
      dialog?.close()
      await load()
      await app.refreshSomas()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  // --- New soma --------------------------------------------------------------

  let createDialog: HTMLDialogElement | undefined = $state()
  let newName = $state('')
  let newOwner = $state(0)
  let newGroup = $state(0)
  let newRole = $state<ShareRole>('viewer')
  const roles = [...SHARE_ROLES].reverse()
  const userName = (u: User) => (u.displayName ? `${u.displayName} (${u.username})` : u.username)

  function openCreate() {
    newName = ''
    newOwner = app.user?.id ?? users[0]?.id ?? 0
    newGroup = 0
    newRole = 'viewer'
    createDialog?.showModal()
  }

  async function create(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    try {
      const s = await admin.createSoma({
        name: newName,
        ownerId: newOwner,
        group: newGroup ? { id: newGroup, role: newRole } : undefined,
      })
      const owner = users.find((u) => u.id === newOwner)
      app.toast(t.somaCreated(s.name, owner?.displayName || owner?.username || ''), 'info')
      createDialog?.close()
      await load()
      await app.refreshSomas()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  // --- Deleted somas ---------------------------------------------------------

  let restoreDialog: HTMLDialogElement | undefined = $state()
  let restoreTarget = $state.raw<admin.DeletedSoma | null>(null)
  let restoreOwner = $state(0)
  const personName = (p: { displayName: string; username: string }) => p.displayName || p.username
  const activeUsers = $derived(users.filter((u) => !u.disabled))

  function openRestore(d: admin.DeletedSoma) {
    restoreTarget = d
    restoreOwner = d.owner?.id ?? app.user?.id ?? 0
    restoreDialog?.showModal()
  }

  async function restore(e: SubmitEvent) {
    e.preventDefault()
    const d = restoreTarget
    if (!d) return
    busy = true
    try {
      await admin.restoreSoma(d.id, restoreOwner)
      app.toast(t.somaRestored(d.name), 'info')
      restoreDialog?.close()
      await load()
      await app.refreshSomas()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  async function purge(d: admin.DeletedSoma) {
    const ok = await app.ask({ title: t.purgeSomaTitle(d.name), message: t.purgeSomaText, confirmLabel: t.purgeSoma, danger: true })
    if (!ok) return
    try {
      await admin.purgeSoma(d.id)
      app.toast(t.somaPurged(d.name), 'info')
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function remove(s: AdminSoma) {
    const ok = await app.ask({ title: t.deleteSomaTitle(s.name), message: t.deleteSomaText, confirmLabel: t.deleteSoma, danger: true })
    if (!ok) return
    try {
      await admin.deleteSoma(s.id)
      await load()
      await app.refreshSomas()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }
</script>

<div class="admin-page">
  <div class="admin-toolbar">
    <input class="admin-filter" type="search" placeholder={t.somaName} aria-label={t.somaName} bind:value={filter} />
    <button type="button" class="admin-btn primary" onclick={openCreate}><Icon name="plus" size={15} /> {t.newSoma}</button>
  </div>
  {#if somas === null}
    <p class="admin-empty">{t.loading}</p>
  {:else}
    <div class="admin-scroll">
      <table class="admin-table">
        <thead>
          <tr>
            <th>{t.somaName}</th>
            <th>{t.owner}</th>
            <th class="num">{t.members}</th>
            <th class="num">{t.size}</th>
            <th>{t.folder}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each shown as s (s.id)}
            <tr>
              <td><strong>{s.name}</strong><div class="admin-faint admin-mono">{s.id}</div></td>
              <td>{s.owner.displayName || s.owner.username}</td>
              <td class="num">{s.members}{#if s.groups}<div class="admin-faint">{t.groupsShared(s.groups)}</div>{/if}</td>
              <td class="num">{formatSize(s.bytes)}<div class="admin-faint">{t.files(s.files)}</div></td>
              <td class="admin-mono admin-muted">{s.path}</td>
              <td>
                <div class="admin-actions">
                  <button type="button" class="admin-btn" disabled={users.length < 2} onclick={() => openTransfer(s)}>{t.transfer}</button>
                  <button type="button" class="admin-btn danger" onclick={() => remove(s)}>{t.deleteSoma}</button>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div>
      <h2 class="section-title">{t.deletedSomas}</h2>
      <p class="admin-faint">{t.deletedSomasText}</p>
    </div>
    {#if deleted.length === 0}
      <p class="admin-faint">{t.noDeletedSomas}</p>
    {:else}
      <div class="admin-scroll">
        <table class="admin-table">
          <thead>
            <tr>
              <th>{t.somaName}</th>
              <th>{t.owner}</th>
              <th>{t.members}</th>
              <th>{t.folder}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each deleted as d (d.id)}
              <tr>
                <td>
                  <strong>{d.name}</strong>
                  <div class="admin-faint">{d.deletedBy ? t.deletedByOn(personName(d.deletedBy), ago(d.deletedAt)) : t.deletedOn(ago(d.deletedAt))}</div>
                </td>
                <td>
                  {#if d.owner}{personName(d.owner)}{:else}<span class="admin-faint">{t.ownerGone}</span>{/if}
                </td>
                <td class="admin-muted">{t.shares(d.members, d.groups)}</td>
                <td class="admin-mono admin-muted">{d.folder}</td>
                <td>
                  <div class="admin-actions">
                    <button type="button" class="admin-btn" onclick={() => openRestore(d)}>{t.restoreSoma}</button>
                    <button type="button" class="admin-btn danger" onclick={() => purge(d)}>{t.purgeSoma}</button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/if}
</div>

<dialog class="admin-dialog" bind:this={restoreDialog} use:closeOnBackdrop aria-labelledby="restore-title">
  <form onsubmit={restore}>
    <h2 id="restore-title">{t.restoreSomaTitle(restoreTarget?.name ?? '')}</h2>
    <p class="admin-faint">{t.restoreSomaText}</p>
    <label for="restore-owner">{t.restoreSomaOwner}</label>
    <select id="restore-owner" class="admin-field" bind:value={restoreOwner}>
      {#each activeUsers as u (u.id)}
        <option value={u.id}>{userName(u)}</option>
      {/each}
    </select>
    <div class="admin-actions">
      <button type="button" class="admin-btn" onclick={() => restoreDialog?.close()}>{strings.somas.cancel}</button>
      <button type="submit" class="admin-btn primary" disabled={busy || !restoreOwner}>{t.restoreSoma}</button>
    </div>
  </form>
</dialog>

<dialog class="admin-dialog" bind:this={dialog} use:closeOnBackdrop aria-labelledby="transfer-title">
  <form onsubmit={transfer}>
    <h2 id="transfer-title">{t.transferTitle(target?.name ?? '')}</h2>
    <label for="transfer-owner">{t.owner}</label>
    <select id="transfer-owner" class="admin-field" bind:value={owner}>
      {#each users.filter((u) => u.id !== target?.ownerId) as u (u.id)}
        <option value={u.id}>{u.displayName || u.username}</option>
      {/each}
    </select>
    <div class="admin-actions">
      <button type="button" class="admin-btn" onclick={() => dialog?.close()}>{t.cancel}</button>
      <button type="submit" class="admin-btn primary" disabled={busy}>{t.transfer}</button>
    </div>
  </form>
</dialog>

<dialog class="admin-dialog" bind:this={createDialog} use:closeOnBackdrop aria-labelledby="new-soma-title">
  <form onsubmit={create}>
    <h2 id="new-soma-title">{t.newSomaTitle}</h2>
    <p>{t.newSomaText}</p>
    <label for="new-soma-name">{t.newSomaName}</label>
    <input id="new-soma-name" class="admin-field" bind:value={newName} maxlength="100" required autocomplete="off" />
    <label for="new-soma-owner">{t.newSomaOwner}</label>
    <select id="new-soma-owner" class="admin-field" bind:value={newOwner}>
      {#each users.filter((u) => !u.disabled) as u (u.id)}
        <option value={u.id}>{userName(u)}</option>
      {/each}
    </select>
    <label for="new-soma-group">{t.newSomaGroup}</label>
    <select id="new-soma-group" class="admin-field" bind:value={newGroup}>
      <option value={0}>{t.newSomaNoGroup}</option>
      {#each groups as g (g.id)}
        <option value={g.id}>{g.everyone ? t.everyone : g.name}</option>
      {/each}
    </select>
    {#if newGroup}
      <label for="new-soma-role">{t.newSomaRole}</label>
      <select id="new-soma-role" class="admin-field" bind:value={newRole} aria-describedby="new-soma-role-hint">
        {#each roles as r (r)}
          <option value={r}>{strings.share.roles[r]}</option>
        {/each}
      </select>
      <p id="new-soma-role-hint" class="hint">{strings.share.roleHints[newRole]}</p>
    {/if}
    <div class="admin-actions">
      <button type="button" class="admin-btn" onclick={() => createDialog?.close()}>{t.cancel}</button>
      <button type="submit" class="admin-btn primary" disabled={busy || !newName.trim() || !newOwner}>{t.createSoma}</button>
    </div>
  </form>
</dialog>

<style>
  .section-title {
    margin: var(--space-4) 0 2px;
    font-size: var(--text-md);
    font-weight: 600;
  }
  .hint {
    margin-top: 6px;
    font-size: var(--text-xs);
  }
</style>
