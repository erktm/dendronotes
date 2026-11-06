<script lang="ts">
  import { app } from '../lib/state/app.svelte'
  import * as admin from '../lib/api/admin'
  import type { AuditEntry } from '../lib/api/admin'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  const t = strings.admin
  const PAGE = 100

  let entries = $state.raw<AuditEntry[] | null>(null)
  let more = $state(false)
  let busy = $state(false)

  async function load(before = 0) {
    busy = true
    try {
      const page = await admin.audit(before, PAGE)
      entries = before ? [...(entries ?? []), ...page] : page
      more = page.length === PAGE
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }
  $effect(() => {
    void load()
  })
</script>

<div class="admin-page">
  {#if entries === null}
    <p class="admin-empty">{t.loading}</p>
  {:else if entries.length === 0}
    <p class="admin-empty">{t.noEntries}</p>
  {:else}
    <div class="admin-scroll">
      <table class="admin-table">
        <thead>
          <tr><th>{t.when}</th><th>{t.who}</th><th>{t.action}</th><th>{t.target}</th><th>{t.detail}</th><th>{t.from}</th></tr>
        </thead>
        <tbody>
          {#each entries as e (e.id)}
            <tr>
              <td class="admin-muted nowrap">{admin.formatDate(e.at)}</td>
              <td>{e.actor}</td>
              <td><span class="admin-pill" class:warn={e.action.includes('failed')}>{e.action}</span></td>
              <td class="admin-mono">{e.target}</td>
              <td class="admin-muted">{e.detail}</td>
              <td class="admin-faint">{e.ip}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if more}
      <div class="admin-toolbar">
        <button type="button" class="admin-btn" disabled={busy} onclick={() => load(entries?.at(-1)?.id ?? 0)}>{t.more}</button>
      </div>
    {/if}
  {/if}
</div>
