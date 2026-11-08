<script lang="ts">
  import Icon from './Icon.svelte'
  import AvatarEditor from './AvatarEditor.svelte'
  import { app } from '../lib/state/app.svelte'
  import * as api from '../lib/api/soma'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  let dialog: HTMLDialogElement | undefined = $state()
  let displayName = $state('')
  let email = $state('')
  let current = $state('')
  let next = $state('')
  let repeat = $state('')
  let busy = $state(false)
  let pwError = $state('')

  $effect(() => {
    if (!dialog) return
    if (app.profileOpen && !dialog.open) {
      displayName = app.user?.displayName ?? ''
      email = app.user?.email ?? ''
      current = next = repeat = pwError = ''
      dialog.showModal()
    } else if (!app.profileOpen && dialog.open) dialog.close()
  })

  const local = $derived(app.user?.source === 'local')
  // Name and e-mail of single sign-on accounts come from the provider.
  const managed = $derived(app.user?.source === 'oidc')

  async function saveProfile(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    try {
      app.setUser(await api.updateProfile({ displayName, email }))
      app.toast(strings.profile.saved, 'info')
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  async function changePassword(e: SubmitEvent) {
    e.preventDefault()
    pwError = ''
    if (next !== repeat) {
      pwError = strings.profile.mismatch
      return
    }
    busy = true
    try {
      await api.changePassword(current, next)
      current = next = repeat = ''
      app.toast(strings.profile.changed, 'info')
    } catch (err) {
      pwError = errorMessage(err)
    } finally {
      busy = false
    }
  }
</script>

<dialog bind:this={dialog} aria-labelledby="profile-title" onclose={() => (app.profileOpen = false)}>
  <header>
    <h2 id="profile-title">{strings.profile.title}</h2>
    <button type="button" class="icon-btn" aria-label={strings.profile.close} onclick={() => (app.profileOpen = false)}>
      <Icon name="close" size={18} />
    </button>
  </header>

  {#if app.user}
    <AvatarEditor user={app.user} />
    <form class="section" onsubmit={saveProfile}>
      <p class="who">
        <strong>{app.user.username}</strong>
        <span class="muted">{strings.profile.signsInVia(strings.profile.sources[app.user.source] ?? app.user.source)}</span>
      </p>
      <label for="profile-name">{strings.profile.displayName}</label>
      <input id="profile-name" bind:value={displayName} maxlength="100" autocomplete="name" readonly={managed} />
      <label for="profile-email">{strings.profile.email}</label>
      <input id="profile-email" type="email" bind:value={email} maxlength="254" autocomplete="email" readonly={managed} />
      {#if managed}
        <p class="hint">{strings.profile.managedByProvider}</p>
      {:else}
        <div class="actions">
          <button type="submit" class="primary" disabled={busy}>{strings.profile.save}</button>
        </div>
      {/if}
    </form>

    {#if local}
      <form class="section" onsubmit={changePassword}>
        <h3>{strings.profile.passwordTitle}</h3>
        <label for="pw-current">{strings.profile.current}</label>
        <input id="pw-current" type="password" bind:value={current} autocomplete="current-password" required />
        <label for="pw-new">{strings.profile.next}</label>
        <input id="pw-new" type="password" bind:value={next} autocomplete="new-password" minlength="10" required aria-describedby="pw-hint" />
        <span class="hint" id="pw-hint">{strings.profile.minLength}</span>
        <label for="pw-repeat">{strings.profile.repeat}</label>
        <input id="pw-repeat" type="password" bind:value={repeat} autocomplete="new-password" minlength="10" required />
        {#if pwError}<p class="error" role="alert">{pwError}</p>{/if}
        <div class="actions">
          <button type="submit" class="primary" disabled={busy || !current || next.length < 10}>{strings.profile.change}</button>
        </div>
      </form>
    {/if}
  {/if}
</dialog>

<style>
  dialog {
    width: min(92vw, 460px);
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
    padding: 16px 16px 16px 20px;
    border-bottom: 1px solid var(--line);
  }
  h2 {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  h3 {
    margin: 0 0 4px;
    font-size: 11.5px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .icon-btn {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .section {
    display: flex;
    flex-direction: column;
    padding: 16px 20px 18px;
  }
  .section + .section {
    border-top: 1px solid var(--line);
  }
  .who {
    display: flex;
    flex-direction: column;
    margin: 0 0 4px;
  }
  .muted,
  .hint {
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  .hint {
    margin-top: 4px;
  }
  label {
    margin: 12px 0 6px;
    font-size: var(--text-sm);
    font-weight: 550;
  }
  input {
    height: 38px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
  }
  input:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
    border-color: transparent;
  }
  .error {
    margin: 8px 0 0;
    color: var(--err);
    font-size: var(--text-sm);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 16px;
  }
  .primary {
    height: 36px;
    padding: 0 var(--space-4);
    border: 0;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--on-accent);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }
  .primary:disabled {
    cursor: not-allowed;
    opacity: 0.5;
  }
</style>
