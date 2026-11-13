<script lang="ts">
  import { onMount } from 'svelte'
  import AppBanners from './components/AppBanners.svelte'
  import AboutLine from './components/AboutLine.svelte'
  import Login from './components/Login.svelte'
  import Logo from './components/Logo.svelte'
  import ResetPassword from './components/ResetPassword.svelte'
  import ConfirmAccount from './components/ConfirmAccount.svelte'
  import Shell from './components/Shell.svelte'
  import Toasts from './components/Toasts.svelte'
  import { app } from './lib/state/app.svelte'
  import { RESET_PATH } from './lib/resetToken'
  import { strings } from './lib/strings'

  // The page of reset and welcome links works without (and apart from) a
  // session, and so does the page of a public link (/p/<token>).
  const resetPage = location.pathname === RESET_PATH
  const confirmPage = location.pathname === '/confirm-account'
  const publicToken = /^\/p\/([A-Za-z0-9_-]{32,64})\/?$/.exec(location.pathname)?.[1] ?? ''

  onMount(() => {
    if (!resetPage && !confirmPage && !publicToken) void app.init()
  })
</script>

{#if publicToken}
  <!-- Loaded on demand: most visits never need it. -->
  {#await import('./components/PublicView.svelte') then m}
    <m.default token={publicToken} />
  {/await}
{:else if resetPage}
  <ResetPassword />
{:else if confirmPage}
  <ConfirmAccount />
{:else if app.auth === 'checking'}
  <div class="splash" aria-label={strings.app.loading}><Logo size={40} /></div>
{:else if app.auth === 'anonymous'}
  <Login />
{:else}
  <div class="frame">
    <AppBanners />
    <div class="frame-main"><Shell /></div>
    <div class="frame-foot"><AboutLine version={app.version} /></div>
  </div>
{/if}

<Toasts />

<style>
  .frame {
    display: flex;
    flex-direction: column;
    height: 100%;
  }
  .frame-main {
    flex: 1;
    min-height: 0;
  }
  .frame-foot {
    flex: none;
    border-top: 1px solid var(--line);
    background: var(--bg-sunken);
  }
  .splash {
    display: grid;
    place-items: center;
    height: 100%;
    opacity: 0.6;
  }
</style>
