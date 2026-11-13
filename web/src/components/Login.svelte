<script lang="ts">
  import AuthCard from './AuthCard.svelte'
  import { app } from '../lib/state/app.svelte'
  import { ApiError } from '../lib/api/client'
  import { authMethods, forgotPassword, signup as requestSignup, type AuthMethods } from '../lib/api/soma'
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
  // "Create an account": the form, then where the link went.
  let signup = $state(false)
  let su = $state({ username: '', displayName: '', email: '', password: '' })
  let signupSentTo = $state('')

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

  function openSignup() {
    signup = true
    signupSentTo = error = ''
  }

  function closeSignup() {
    signup = false
    signupSentTo = error = ''
    su.password = ''
  }

  async function sendSignup(e: SubmitEvent) {
    e.preventDefault()
    if (busy) return
    busy = true
    error = ''
    try {
      await requestSignup({ username: su.username.trim(), displayName: su.displayName.trim(), email: su.email.trim(), password: su.password })
      signupSentTo = su.email.trim()
      su.password = ''
    } catch (err) {
      error = rateLimited(err) || errorMessage(err)
    } finally {
      busy = false
    }
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
  {:else if signup}
    <h1>{strings.auth.signupTitle}</h1>
    {#if signupSentTo}
      <p class="lead" role="status">{strings.auth.signupSent(signupSentTo)}</p>
      <button class="primary" type="button" onclick={closeSignup}>{strings.auth.backToSignIn}</button>
    {:else}
      <p class="lead">{strings.auth.signupLead}</p>
      <form onsubmit={sendSignup}>
        <label for="signup-username">{strings.auth.signupUsername}</label>
        <input
          id="signup-username"
          type="text"
          autocomplete="username"
          autocapitalize="none"
          spellcheck="false"
          minlength="2"
          maxlength="64"
          bind:value={su.username}
          use:focusOnMount
          aria-describedby="signup-username-hint"
          required
        />
        <span class="hint" id="signup-username-hint">{strings.auth.signupUsernameHint}</span>
        <label for="signup-name">{strings.auth.signupDisplayName}</label>
        <input id="signup-name" type="text" autocomplete="name" maxlength="100" bind:value={su.displayName} />
        <label for="signup-email">{strings.auth.signupEmail}</label>
        <input id="signup-email" type="email" autocomplete="email" bind:value={su.email} required />
        <label for="signup-password">{strings.auth.signupPassword}</label>
        <input
          id="signup-password"
          type="password"
          autocomplete="new-password"
          minlength="10"
          bind:value={su.password}
          aria-describedby="signup-password-hint"
          required
        />
        <span class="hint" id="signup-password-hint">{strings.auth.resetMinLength}</span>
        {#if error}<p class="error" role="alert">{error}</p>{/if}
        <button class="primary" type="submit" disabled={busy || su.username.trim().length < 2 || !su.email.trim() || su.password.length < 10}>
          {busy ? strings.auth.signupSending : strings.auth.signupSend}
        </button>
        <button class="link" type="button" onclick={closeSignup}>{strings.auth.backToSignIn}</button>
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
    {#if methods?.signup}
      <p class="signup">
        {strings.auth.noAccount}
        <button class="link inline" type="button" onclick={openSignup}>{strings.auth.signupOpen}</button>
      </p>
    {/if}
  {/if}
</AuthCard>

<style>
  .signup {
    margin-top: var(--space-4);
    color: var(--text-muted);
    font-size: var(--text-sm);
    text-align: center;
  }
  .signup .inline {
    display: inline;
    width: auto;
    margin: 0;
    padding: 0;
  }
</style>
