<script lang="ts">
  import AdminUsers from './AdminUsers.svelte'
  import AdminSomas from './AdminSomas.svelte'
  import AdminSessions from './AdminSessions.svelte'
  import AdminServer from './AdminServer.svelte'
  import AdminAudit from './AdminAudit.svelte'
  import EmptyState from './EmptyState.svelte'
  import { app } from '../lib/state/app.svelte'
  import { router } from '../lib/router.svelte'
  import { strings } from '../lib/strings'

  const tabs = ['users', 'somas', 'sessions', 'server', 'audit'] as const
  const tab = $derived((tabs as readonly string[]).includes(router.adminTab) ? router.adminTab : 'users')
</script>

{#if !app.isAdmin}
  <EmptyState title={strings.errors.byCode.forbidden} text="" />
{:else}
  <section class="admin" aria-labelledby="admin-title">
    <header>
      <h1 id="admin-title">{strings.admin.title}</h1>
      <nav aria-label={strings.admin.title}>
        {#each tabs as t (t)}
          <button type="button" aria-current={tab === t ? 'page' : undefined} onclick={() => router.goAdmin(t)}>
            {strings.admin.tabs[t]}
          </button>
        {/each}
      </nav>
    </header>
    <div class="body">
      {#if tab === 'users'}
        <AdminUsers />
      {:else if tab === 'somas'}
        <AdminSomas />
      {:else if tab === 'sessions'}
        <AdminSessions />
      {:else if tab === 'server'}
        <AdminServer />
      {:else}
        <AdminAudit />
      {/if}
    </div>
  </section>
{/if}

<style>
  .admin {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  header {
    flex: none;
    padding: var(--space-6) var(--space-6) 0;
    border-bottom: 1px solid var(--line);
  }
  h1 {
    margin: 0 0 var(--space-3);
    font-size: var(--text-xl);
    font-weight: 650;
  }
  nav {
    display: flex;
    gap: 2px;
    overflow-x: auto;
  }
  nav button {
    height: 36px;
    padding: 0 12px;
    border: 0;
    border-bottom: 2px solid transparent;
    background: transparent;
    color: var(--text-muted);
    font: inherit;
    font-size: var(--text-sm);
    font-weight: 550;
    white-space: nowrap;
    cursor: pointer;
  }
  nav button[aria-current='page'] {
    border-bottom-color: var(--accent);
    color: var(--text);
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }
</style>
