<script lang="ts">
  import { onMount } from 'svelte'
  import AuthCard from './AuthCard.svelte'
  import { ApiError } from '../lib/api/client'
  import { app } from '../lib/state/app.svelte'
  import { errorMessage } from '../lib/errors'
  import { takeResetToken } from '../lib/resetToken'
  import { strings } from '../lib/strings'

  // The page behind the link in a sign-up e-mail (/confirm-account#<token>).
  // The token leaves the address bar at once; on success the account is
  // signed in and the app opens.
  const token = takeResetToken()
  const t = strings.auth

  let problem = $state(token ? '' : t.confirmMissing)

  onMount(async () => {
    document.title = t.confirmPageTitle
    if (!token) return
    try {
      await app.confirmSignup(token)
      location.replace('/')
    } catch (err) {
      if (err instanceof ApiError && err.code === 'invalid_token') problem = t.confirmInvalid
      else if (err instanceof ApiError && (err.code === 'username_taken' || err.code === 'email_taken')) problem = t.confirmTaken
      else if (err instanceof ApiError && err.code === 'not_available') problem = t.confirmClosed
      else problem = errorMessage(err)
    }
  })
</script>

<AuthCard>
  <h1>{t.confirmTitle}</h1>
  {#if problem}
    <p class="lead" role="alert">{problem}</p>
    <a class="primary" href="/">{t.backToSignIn}</a>
  {:else}
    <p class="lead" role="status">{t.confirmWorking}</p>
  {/if}
</AuthCard>
