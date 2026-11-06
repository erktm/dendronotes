<script lang="ts">
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import * as admin from '../lib/api/admin'
  import type { User } from '../lib/api/soma'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  const t = strings.admin

  let users = $state.raw<User[] | null>(null)
  let filter = $state('')

  async function load() {
    try {
      users = await admin.users()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }
  $effect(() => {
    void load()
  })

  const shown = $derived.by(() => {
    const q = filter.trim().toLowerCase()
    return (users ?? []).filter(
      (u) => !q || u.username.toLowerCase().includes(q) || u.displayName.toLowerCase().includes(q) || u.email.toLowerCase().includes(q),
    )
  })

  // --- Dialog ----------------------------------------------------------------

  type Mode = 'create' | 'edit' | 'password' | 'delete'
  let dialog: HTMLDialogElement | undefined = $state()
  let mode = $state<Mode>('create')
  let target = $state.raw<User | null>(null)
  let form = $state({ username: '', displayName: '', email: '', password: '', role: 'user' as 'admin' | 'user', transferTo: 0 })
  let busy = $state(false)
  let formError = $state('')

  function openDialog(m: Mode, u: User | null) {
    mode = m
    target = u
    formError = ''
    form = {
      username: u?.username ?? '',
      displayName: u?.displayName ?? '',
      email: u?.email ?? '',
      password: '',
      role: u?.role ?? 'user',
      transferTo: app.user?.id ?? 0,
    }
    dialog?.showModal()
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    formError = ''
    try {
      if (mode === 'create') {
        const { username, displayName, email, password, role } = form
        const u = await admin.createUser({ username, displayName, email, password, role })
        app.toast(t.userCreated(u.username), 'info')
      } else if (mode === 'edit' && target) {
        await admin.updateUser(target.id, { displayName: form.displayName, email: form.email, role: form.role })
        app.toast(t.userSaved(target.username), 'info')
      } else if (mode === 'password' && target) {
        await admin.setPassword(target.id, form.password)
        app.toast(t.passwordSet(target.username), 'info')
      } else if (mode === 'delete' && target) {
        await admin.deleteUser(target.id, form.transferTo || undefined)
        app.toast(t.userDeleted(target.username), 'info')
        void app.refreshSomas()
      }
      dialog?.close()
      await load()
    } catch (err) {
      formError = errorMessage(err)
    } finally {
      busy = false
    }
  }

  async function toggleDisabled(u: User) {
    try {
      await admin.updateUser(u.id, { disabled: !u.disabled })
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function endSessions(u: User) {
    try {
      await admin.endUserSessions(u.id)
      app.toast(t.sessionsEnded(u.username), 'info')
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  const title = $derived(
    mode === 'create'
      ? t.newUser
      : mode === 'edit'
        ? t.editUser(target?.username ?? '')
        : mode === 'password'
          ? t.resetTitle(target?.username ?? '')
          : t.deleteTitle(target?.username ?? ''),
  )
</script>

<div class="admin-page">
  <div class="admin-toolbar">
    <input class="admin-filter" type="search" placeholder={t.searchUsers} aria-label={t.searchUsers} bind:value={filter} />
    <button type="button" class="admin-btn primary" onclick={() => openDialog('create', null)}>
      <Icon name="plus" size={15} />
      {t.newUser}
    </button>
  </div>

  {#if users === null}
    <p class="admin-empty">{t.loading}</p>
  {:else}
    <div class="admin-scroll">
      <table class="admin-table">
        <thead>
          <tr>
            <th>{t.username}</th>
            <th>{t.role}</th>
            <th>{t.source}</th>
            <th>{t.status}</th>
            <th>{t.lastLogin}</th>
            <th><span class="visually-hidden">{t.actions('')}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each shown as u (u.id)}
            {@const me = u.id === app.user?.id}
            <tr>
              <td>
                <strong>{u.displayName || u.username}</strong>
                {#if me}<span class="admin-pill accent">{t.you}</span>{/if}
                <div class="admin-faint">{u.username}{u.email ? ` · ${u.email}` : ''}</div>
              </td>
              <td><span class="admin-pill" class:accent={u.role === 'admin'}>{u.role === 'admin' ? t.roleAdmin : t.roleUser}</span></td>
              <td class="admin-muted">{strings.profile.sources[u.source] ?? u.source}</td>
              <td><span class="admin-pill" class:warn={u.disabled}>{u.disabled ? t.disabled : t.active}</span></td>
              <td class="admin-muted nowrap">{admin.formatDate(u.lastLoginAt) || t.never}</td>
              <td>
                <div class="admin-actions" role="group" aria-label={t.actions(u.username)}>
                  <button type="button" class="admin-btn" onclick={() => openDialog('edit', u)}>{t.edit}</button>
                  {#if u.source === 'local'}
                    <button type="button" class="admin-btn" onclick={() => openDialog('password', u)}>{t.resetPassword}</button>
                  {/if}
                  <button type="button" class="admin-btn" onclick={() => endSessions(u)}>{t.endSessions}</button>
                  {#if !me}
                    <button type="button" class="admin-btn" onclick={() => toggleDisabled(u)}>{u.disabled ? t.enable : t.disable}</button>
                    <button type="button" class="admin-btn danger" onclick={() => openDialog('delete', u)}>{t.deleteUser}</button>
                  {/if}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<dialog class="admin-dialog" bind:this={dialog} aria-labelledby="user-dialog-title">
  <form onsubmit={submit}>
    <h2 id="user-dialog-title">{title}</h2>
    {#if mode === 'create' || mode === 'edit'}
      {#if mode === 'create'}
        <label for="u-name">{t.username}</label>
        <input id="u-name" class="admin-field" bind:value={form.username} required minlength="2" maxlength="64" autocomplete="off" spellcheck="false" />
      {/if}
      <label for="u-display">{t.displayName}</label>
      <input id="u-display" class="admin-field" bind:value={form.displayName} maxlength="100" autocomplete="off" />
      <label for="u-email">{t.email}</label>
      <input id="u-email" class="admin-field" type="email" bind:value={form.email} maxlength="254" autocomplete="off" />
      <label for="u-role">{t.role}</label>
      <select id="u-role" class="admin-field" bind:value={form.role}>
        <option value="user">{t.roleUser}</option>
        <option value="admin">{t.roleAdmin}</option>
      </select>
      {#if mode === 'create'}
        <label for="u-pw">{t.initialPassword}</label>
        <input id="u-pw" class="admin-field" type="password" bind:value={form.password} required minlength="10" autocomplete="new-password" />
        <p>{strings.profile.minLength}</p>
      {/if}
    {:else if mode === 'password'}
      <p>{t.resetText}</p>
      <label for="u-newpw">{strings.profile.next}</label>
      <input id="u-newpw" class="admin-field" type="password" bind:value={form.password} required minlength="10" autocomplete="new-password" />
      <p>{strings.profile.minLength}</p>
    {:else}
      <p>{t.deleteText}</p>
      <label for="u-heir">{t.transferTo}</label>
      <select id="u-heir" class="admin-field" bind:value={form.transferTo}>
        {#each (users ?? []).filter((x) => x.id !== target?.id) as x (x.id)}
          <option value={x.id}>{x.displayName || x.username}</option>
        {/each}
        <option value={0}>{t.transferNone}</option>
      </select>
    {/if}
    {#if formError}<p class="error" role="alert">{formError}</p>{/if}
    <div class="admin-actions">
      <button type="button" class="admin-btn" onclick={() => dialog?.close()}>{t.cancel}</button>
      <button type="submit" class="admin-btn {mode === 'delete' ? 'danger' : 'primary'}" disabled={busy}>
        {mode === 'create' ? t.createUser : mode === 'delete' ? t.deleteUser : mode === 'password' ? t.resetPassword : t.save}
      </button>
    </div>
  </form>
</dialog>

<style>
  .error {
    margin-top: 10px !important;
    color: var(--err) !important;
  }
  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
  strong + :global(.admin-pill) {
    margin-left: 6px;
  }
</style>
