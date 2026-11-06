<script lang="ts">
  import { app } from '../lib/state/app.svelte'
  import * as admin from '../lib/api/admin'
  import type { Session } from '../lib/api/admin'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  const t = strings.admin

  let sessions = $state.raw<Session[] | null>(null)
  let current = $state(0)

  async function load() {
    try {
      const r = await admin.sessions()
      sessions = r.sessions
      current = r.current
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }
  $effect(() => {
    void load()
  })

  async function end(s: Session) {
    try {
      await admin.endSession(s.id)
      app.toast(t.ended, 'info')
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  /** "Firefox on macOS"-ish summary of a user agent. */
  function client(ua: string): string {
    const browser = /Firefox|Edg|Chrome|Safari/.exec(ua)?.[0]?.replace('Edg', 'Edge') ?? ''
    const os = /Windows|Mac OS X|Android|iPhone|iPad|Linux/.exec(ua)?.[0]?.replace('Mac OS X', 'macOS') ?? ''
    return [browser, os].filter(Boolean).join(' · ') || ua.slice(0, 40)
  }
</script>

<div class="admin-page">
  {#if sessions === null}
    <p class="admin-empty">{t.loading}</p>
  {:else if sessions.length === 0}
    <p class="admin-empty">{t.noSessions}</p>
  {:else}
    <div class="admin-scroll">
      <table class="admin-table">
        <thead>
          <tr>
            <th>{t.user}</th>
            <th>{t.started}</th>
            <th>{t.lastSeen}</th>
            <th>{t.expires}</th>
            <th>{t.client}</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each sessions as s (s.id)}
            <tr>
              <td>
                <strong>{s.username}</strong>
                {#if s.id === current}<span class="admin-pill accent">{t.thisSession}</span>{/if}
              </td>
              <td class="admin-muted nowrap">{admin.formatDate(s.createdAt)}</td>
              <td class="admin-muted nowrap">{admin.formatDate(s.seenAt)}</td>
              <td class="admin-muted nowrap">{admin.formatDate(s.expiresAt)}</td>
              <td class="admin-muted" title={s.userAgent}>{client(s.userAgent)}<div class="admin-faint">{s.ip}</div></td>
              <td>
                <div class="admin-actions">
                  <button type="button" class="admin-btn danger" disabled={s.id === current} onclick={() => end(s)}>{t.end}</button>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
