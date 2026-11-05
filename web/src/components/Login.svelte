<script lang="ts">
  import Logo from './Logo.svelte'
  import { app } from '../lib/state/app.svelte'
  import { ApiError } from '../lib/api/client'
  import { errorMessage } from '../lib/errors'
  import { focusOnMount } from '../lib/actions'

  let password = $state('')
  let busy = $state(false)
  let error = $state('')

  $effect(() => {
    document.title = 'Sign in – leafnotes'
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!password || busy) return
    busy = true
    error = ''
    try {
      await app.login(password)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        error = 'That password isn’t correct.'
      } else if (err instanceof ApiError && err.status === 429) {
        const secs = typeof err.body.retryAfter === 'number' ? err.body.retryAfter : 0
        error = secs ? `Too many attempts. Try again in ${secs} seconds.` : errorMessage(err)
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
      <span class="wordmark">leaf<span>notes</span></span>
    </div>

    {#if app.mode === 'proxy'}
      <h1>You’re not signed in</h1>
      <p class="lead">
        This server expects your reverse proxy (for example oauth2-proxy) to sign you in, but it didn’t
        receive a user. Reload to try again, or ask your administrator.
      </p>
      <button class="primary" type="button" onclick={() => location.reload()}>Reload</button>
    {:else}
      <h1>Welcome back</h1>
      <p class="lead">Sign in to open your vault.</p>
      <form onsubmit={submit}>
        <!-- Helps password managers that expect a user name. -->
        <input class="visually-hidden" type="text" name="username" autocomplete="username" value="leafnotes" readonly tabindex="-1" aria-hidden="true" />
        <label for="password">Password</label>
        <input
          id="password"
          name="password"
          type="password"
          autocomplete="current-password"
          bind:value={password}
          use:focusOnMount
          aria-invalid={error ? 'true' : undefined}
          aria-describedby={error ? 'login-error' : undefined}
          required
        />
        {#if error}
          <p class="error" id="login-error" role="alert">{error}</p>
        {/if}
        <button class="primary" type="submit" disabled={busy || !password}>
          {busy ? 'Signing in …' : 'Sign in'}
        </button>
      </form>
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
  .wordmark {
    font-size: 1.5rem;
    font-weight: 650;
    letter-spacing: -0.02em;
    color: var(--accent);
  }
  .wordmark span {
    color: var(--text);
    font-weight: 500;
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
  input[type='password'] {
    height: 42px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
  }
  input[type='password']:focus-visible {
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
  .primary:disabled {
    opacity: 0.55;
    cursor: default;
  }
</style>
