<script lang="ts">
  import { onMount } from 'svelte'
  import Login from './components/Login.svelte'
  import Logo from './components/Logo.svelte'
  import ResetPassword from './components/ResetPassword.svelte'
  import Shell from './components/Shell.svelte'
  import Toasts from './components/Toasts.svelte'
  import { app } from './lib/state/app.svelte'
  import { RESET_PATH } from './lib/resetToken'
  import { strings } from './lib/strings'

  // The page of reset and welcome links works without (and apart from) a
  // session.
  const resetPage = location.pathname === RESET_PATH

  onMount(() => {
    if (!resetPage) void app.init()
  })
</script>

{#if resetPage}
  <ResetPassword />
{:else if app.auth === 'checking'}
  <div class="splash" aria-label={strings.app.loading}><Logo size={40} /></div>
{:else if app.auth === 'anonymous'}
  <Login />
{:else}
  <Shell />
{/if}

<Toasts />

<style>
  .splash {
    display: grid;
    place-items: center;
    height: 100%;
    opacity: 0.6;
  }
</style>
