<script lang="ts">
  import { closeOnBackdrop } from '../lib/actions'
  import { app } from '../lib/state/app.svelte'
  import * as admin from '../lib/api/admin'
  import type { AdminSoma } from '../lib/api/admin'
  import type { User } from '../lib/api/soma'
  import { formatSize } from '../lib/files'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  const t = strings.admin

  let somas = $state.raw<AdminSoma[] | null>(null)
  let users = $state.raw<User[]>([])
  let filter = $state('')

  async function load() {
    try {
      ;[somas, users] = await Promise.all([admin.somas(), admin.users()])
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
              <td class="num">{s.members}</td>
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
  {/if}
</div>

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
