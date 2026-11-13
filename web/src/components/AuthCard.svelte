<script lang="ts">
  import type { Snippet } from 'svelte'
  import LoginBanners from './LoginBanners.svelte'
  import Logo from './Logo.svelte'
  import Wordmark from './Wordmark.svelte'

  // The card of the pages before signing in (sign-in, choosing a password).
  let { children }: { children: Snippet } = $props()
</script>

<main class="screen">
  <LoginBanners />
  <div class="card auth-card">
    <div class="brand">
      <Logo size={44} />
      <Wordmark size="lg" />
    </div>
    {@render children()}
  </div>
</main>

<style>
  .screen {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-4);
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
  .auth-card :global(h1) {
    font-size: var(--text-xl);
    font-weight: 600;
    letter-spacing: -0.015em;
  }
  .auth-card :global(.lead) {
    margin: var(--space-2) 0 var(--space-6);
    color: var(--text-muted);
  }
  .auth-card :global(form) {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .auth-card :global(label) {
    font-size: var(--text-sm);
    font-weight: 550;
  }
  .auth-card :global(label:not(:first-child)) {
    margin-top: var(--space-2);
  }
  .auth-card :global(input[type='password']),
  .auth-card :global(input[type='text']) {
    height: 42px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
  }
  .auth-card :global(input[type='password']:focus-visible),
  .auth-card :global(input[type='text']:focus-visible) {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
    border-color: transparent;
  }
  .auth-card :global(.hint) {
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  .auth-card :global(.error) {
    color: var(--err);
    font-size: var(--text-sm);
  }
  .auth-card :global(.done) {
    margin: var(--space-2) 0 var(--space-4);
  }
  .auth-card :global(.primary) {
    width: 100%;
    margin-top: var(--space-3);
    height: 42px;
    border: 0;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--on-accent);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }
  .auth-card :global(.primary:hover:not(:disabled)) {
    background: var(--accent-strong);
  }
  .auth-card :global(a.primary) {
    display: grid;
    place-items: center;
    margin-top: 0;
    text-decoration: none;
  }
  .auth-card :global(.primary:disabled) {
    opacity: 0.55;
    cursor: default;
  }
  .auth-card :global(.sso + .error) {
    margin-top: var(--space-3);
  }
  .auth-card :global(.link) {
    align-self: flex-start;
    margin-top: var(--space-3);
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-muted);
    font: inherit;
    font-size: var(--text-sm);
    text-decoration: underline;
    text-underline-offset: 3px;
    cursor: pointer;
  }
  .auth-card :global(.link:hover) {
    color: var(--text);
  }
  .auth-card :global(.or) {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin: var(--space-4) 0;
    color: var(--text-faint);
    font-size: var(--text-xs);
  }
  .auth-card :global(.or::before),
  .auth-card :global(.or::after) {
    content: '';
    flex: 1;
    border-top: 1px solid var(--line);
  }
</style>
