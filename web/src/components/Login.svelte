<script lang="ts">
  import Logo from './Logo.svelte'
  import { app } from '../lib/state/app.svelte'
  import { ApiError } from '../lib/api/client'
  import { authMethods, type AuthMethods } from '../lib/api/soma'
  import { errorMessage } from '../lib/errors'
  import { focusOnMount } from '../lib/actions'
  import { strings } from '../lib/strings'
  import Wordmark from './Wordmark.svelte'

  let username = $state('')
  let password = $state('')
  let busy = $state(false)
  let error = $state('')
  let methods = $state.raw<AuthMethods | null>(null)

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
      } else if (err instanceof ApiError && err.status === 429) {
        const secs = typeof err.body.retryAfter === 'number' ? err.body.retryAfter : 0
        error = secs ? strings.auth.tooManyAttempts(secs) : errorMessage(err)
      } else {
        error = errorMessage(err)
      }
      password = ''
    } finally {
      busy = false
    }
  }
</script>

<main class="screen">
  <div class="card">
    <div class="brand">
      <Logo size={44} />
      <Wordmark size="lg" />
    </div>

    {#if app.mode === 'proxy'}
      <h1>{strings.auth.proxyTitle}</h1>
      <p class="lead">{strings.auth.proxyText}</p>
      <button class="primary" type="button" onclick={() => location.reload()}>{strings.auth.reload}</button>
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
      </form>
      {/if}
    {/if}
  </div>
</main>

<style>
  .screen {
    display: grid;
    place-items: center;
    min-height: 100%;
    padding: var(--space-6) var(--space-4);
    background:
      radial-gradient(60rem 30rem at 15% -10%, var(--accent-soft), transparent 60%),
      var(--bg);
  }
  .card {
    width: min(100%, 380px);
    padding: var(--space-8);
    border: 1px solid var(--line);
    border-radius: 16px;
    background: var(--bg-raised);
    box-shadow: var(--shadow);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-6);
  }
  h1 {
    font-size: var(--text-xl);
    font-weight: 600;
    letter-spacing: -0.015em;
  }
  .lead {
    margin: var(--space-2) 0 var(--space-6);
    color: var(--text-muted);
  }
  form {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  label {
    font-size: var(--text-sm);
    font-weight: 550;
  }
  input[type='password'],
  input[type='text'] {
    height: 42px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
  }
  label[for='password'] {
    margin-top: var(--space-2);
  }
  input[type='password']:focus-visible,
  input[type='text']:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
    border-color: transparent;
  }
  .error {
    color: var(--err);
    font-size: var(--text-sm);
  }
  .primary {
    margin-top: var(--space-3);
    height: 42px;
    border: 0;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--on-accent);
    font-weight: 600;
    cursor: pointer;
  }
  .primary:hover:not(:disabled) {
    background: var(--accent-strong);
  }
  a.primary {
    display: grid;
    place-items: center;
    margin-top: 0;
    text-decoration: none;
  }
  .sso + .error {
    margin-top: var(--space-3);
  }
  .or {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin: var(--space-4) 0;
    color: var(--text-faint);
    font-size: var(--text-xs);
  }
  .or::before,
  .or::after {
    content: '';
    flex: 1;
    border-top: 1px solid var(--line);
  }
  .primary:disabled {
    opacity: 0.55;
    cursor: default;
  }
</style>
