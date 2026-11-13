<script lang="ts">
  import AuthCard from './AuthCard.svelte'
  import { app } from '../lib/state/app.svelte'
  import { ApiError } from '../lib/api/client'
  import { authMethods, forgotPassword, type AuthMethods } from '../lib/api/soma'
  import { errorMessage } from '../lib/errors'
  import { focusOnMount } from '../lib/actions'
  import { strings } from '../lib/strings'

  let username = $state('')
  let password = $state('')
  let busy = $state(false)
  let error = $state('')
  let methods = $state.raw<AuthMethods | null>(null)
  // "Forgot password": the form, then the confirmation for this login.
  let forgot = $state(false)
  let forgotLogin = $state('')
  let sentFor = $state('')

  // ?local=1 shows the password form even when single sign-on replaces it
  // (emergency access for the admin account).
  const params = new URLSearchParams(location.search)
  const forceLocal = params.get('local') === '1'

  // The single sign-on callback reports problems as ?login_error=…
  const loginError = params.get('login_error')
  if (loginError) {
    error = strings.auth.loginErrors[loginError] ?? strings.auth.loginErrors.sso_failed!
    params.delete('login_error')
    const rest = params.toString()
    history.replaceState(history.state, '', location.pathname + (rest ? `?${rest}` : '') + location.hash)
  }

  const showForm = $derived(methods !== null && (methods.local || forceLocal))
  // Return to the page that asked for the sign-in.
  const ssoHref = $derived(`/api/auth/oidc/login?next=${encodeURIComponent(location.pathname + location.search)}`)

  $effect(() => {
    document.title = strings.auth.pageTitle
  })

  $effect(() => {
    authMethods().then(
      (m) => (methods = m),
      () => (methods = { mode: app.mode, local: true, sso: null }),
    )
  })

  function rateLimited(err: unknown): string {
    if (err instanceof ApiError && err.status === 429) {
      const secs = typeof err.body.retryAfter === 'number' ? err.body.retryAfter : 0
      return secs ? strings.auth.tooManyAttempts(secs) : errorMessage(err)
    }
    return ''
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!username || !password || busy) return
    busy = true
    error = ''
    try {
      await app.login(username.trim(), password)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        error = strings.auth.wrongPassword
      } else if (err instanceof ApiError && err.code === 'account_disabled') {
        error = strings.auth.accountDisabled
      } else {
        error = rateLimited(err) || errorMessage(err)
      }
      password = ''
    } finally {
      busy = false
    }
  }

  function openForgot() {
    forgot = true
    forgotLogin = username.trim()
    sentFor = error = ''
  }

  function closeForgot() {
    forgot = false
    sentFor = error = ''
  }

  async function sendLink(e: SubmitEvent) {
    e.preventDefault()
    const login = forgotLogin.trim()
    if (!login || busy) return
    busy = true
    error = ''
    try {
      await forgotPassword(login)
      sentFor = login
    } catch (err) {
      error = rateLimited(err) || errorMessage(err)
    } finally {
      busy = false
    }
  }
</script>

<AuthCard>
  {#if app.mode === 'proxy'}
    <h1>{strings.auth.proxyTitle}</h1>
    <p class="lead">{strings.auth.proxyText}</p>
    <button class="primary" type="button" onclick={() => location.reload()}>{strings.auth.reload}</button>
  {:else if forgot}
    <h1>{strings.auth.forgotTitle}</h1>
    {#if sentFor}
      <p class="lead" role="status">{strings.auth.forgotSent(sentFor)}</p>
      <button class="primary" type="button" onclick={closeForgot}>{strings.auth.backToSignIn}</button>
    {:else}
      <p class="lead">{strings.auth.forgotLead}</p>
      <form onsubmit={sendLink}>
        <label for="forgot-login">{strings.auth.forgotLogin}</label>
        <input
          id="forgot-login"
          type="text"
          autocomplete="username"
          autocapitalize="none"
          spellcheck="false"
          bind:value={forgotLogin}
          use:focusOnMount
          required
        />
        {#if error}<p class="error" role="alert">{error}</p>{/if}
        <button class="primary" type="submit" disabled={busy || !forgotLogin.trim()}>
          {busy ? strings.auth.forgotSending : strings.auth.forgotSend}
        </button>
        <button class="link" type="button" onclick={closeForgot}>{strings.auth.backToSignIn}</button>
      </form>
    {/if}
  {:else}
    <h1>{strings.auth.welcome}</h1>
    <p class="lead">{strings.auth.lead}</p>
    {#if methods?.sso}
      <a class="primary sso" href={ssoHref}>{strings.auth.ssoContinue(methods.sso.label)}</a>
      {#if showForm}<p class="or"><span>{strings.auth.or}</span></p>{/if}
    {/if}
    {#if error && !showForm}
      <p class="error" role="alert">{error}</p>
    {/if}
    {#if showForm}
      <form onsubmit={submit}>
        <label for="username">{strings.auth.username}</label>
        <input
          id="username"
          name="username"
          type="text"
          autocomplete="username"
          autocapitalize="none"
          spellcheck="false"
          bind:value={username}
          use:focusOnMount
          aria-invalid={error ? 'true' : undefined}
          required
        />
        <label for="password">{strings.auth.password}</label>
        <input
          id="password"
          name="password"
          type="password"
          autocomplete="current-password"
          bind:value={password}
          aria-invalid={error ? 'true' : undefined}
          aria-describedby={error ? 'login-error' : undefined}
          required
        />
        {#if error}
          <p class="error" id="login-error" role="alert">{error}</p>
        {/if}
        <button class="primary" type="submit" disabled={busy || !username || !password}>
          {busy ? strings.auth.signingIn : strings.auth.signIn}
        </button>
        {#if methods?.passwordReset}
          <button class="link" type="button" onclick={openForgot}>{strings.auth.forgot}</button>
        {/if}
      </form>
    {/if}
  {/if}
</AuthCard>
