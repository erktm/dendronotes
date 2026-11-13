<script lang="ts">
  import AuthCard from './AuthCard.svelte'
  import { ApiError } from '../lib/api/client'
  import { resetPassword } from '../lib/api/soma'
  import { errorMessage } from '../lib/errors'
  import { focusOnMount } from '../lib/actions'
  import { takeResetToken } from '../lib/resetToken'
  import { strings } from '../lib/strings'

  // The page behind the links in reset and welcome e-mails
  // (/reset-password#<token>). The token leaves the address bar at once.
  const token = takeResetToken()

  let next = $state('')
  let repeat = $state('')
  let busy = $state(false)
  let error = $state('')
  let invalid = $state(!token)
  let doneFor = $state('')

  $effect(() => {
    document.title = strings.auth.resetPageTitle
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    if (next !== repeat) {
      error = strings.auth.resetMismatch
      return
    }
    busy = true
    try {
      doneFor = (await resetPassword(token, next)).username
    } catch (err) {
      if (err instanceof ApiError && err.code === 'invalid_token') invalid = true
      else error = errorMessage(err)
    } finally {
      busy = false
    }
  }
</script>

<AuthCard>
  <h1>{strings.auth.resetTitle}</h1>
  {#if doneFor}
    <p class="lead" role="status">{strings.auth.resetDone(doneFor)}</p>
    <a class="primary" href="/">{strings.auth.signIn}</a>
  {:else if invalid}
    <p class="lead" role="alert">{token ? strings.auth.resetInvalid : strings.auth.resetMissing}</p>
    <a class="primary" href="/">{strings.auth.backToSignIn}</a>
  {:else}
    <p class="lead">{strings.auth.resetLead}</p>
    <form onsubmit={submit}>
      <label for="reset-new">{strings.auth.resetNew}</label>
      <input
        id="reset-new"
        type="password"
        autocomplete="new-password"
        minlength="10"
        bind:value={next}
        use:focusOnMount
        aria-describedby="reset-hint"
        required
      />
      <span class="hint" id="reset-hint">{strings.auth.resetMinLength}</span>
      <label for="reset-repeat">{strings.auth.resetRepeat}</label>
      <input id="reset-repeat" type="password" autocomplete="new-password" minlength="10" bind:value={repeat} required />
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <button class="primary" type="submit" disabled={busy || next.length < 10 || !repeat}>
        {busy ? strings.auth.resetSaving : strings.auth.resetSave}
      </button>
    </form>
  {/if}
</AuthCard>
