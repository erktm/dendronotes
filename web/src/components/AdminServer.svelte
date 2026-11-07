<script lang="ts">
  import { app } from '../lib/state/app.svelte'
  import * as admin from '../lib/api/admin'
  import type { ServerInfo } from '../lib/api/admin'
  import { formatSize } from '../lib/files'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  const t = strings.admin

  let info = $state.raw<ServerInfo | null>(null)

  $effect(() => {
    admin.server().then(
      (i) => (info = i),
      (err) => app.toast(errorMessage(err)),
    )
  })

  const settings = $derived(info ? Object.entries(info.settings).sort(([a], [b]) => a.localeCompare(b)) : [])

  let checking = $state(false)
  async function check() {
    if (!info) return
    checking = true
    try {
      info = { ...info, sso: await admin.checkSSO() }
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      checking = false
    }
  }
</script>

<div class="admin-page">
  {#if !info}
    <p class="admin-empty">{t.loading}</p>
  {:else}
    <div class="admin-cards">
      <div class="admin-card">
        <h3>dendronotes</h3>
        <dl>
          <dt>{t.version}</dt>
          <dd>{info.version}</dd>
          <dt>{t.uptime}</dt>
          <dd>{info.uptime}</dd>
          <dt>{t.platform}</dt>
          <dd>{info.platform} · {info.goVersion}</dd>
          <dt>{t.authMode}</dt>
          <dd>{info.authMode}</dd>
        </dl>
      </div>
      <div class="admin-card">
        <h3>{t.counts}</h3>
        <dl>
          <dt>{t.tabs.users}</dt>
          <dd>{info.counts.users}</dd>
          <dt>{t.tabs.somas}</dt>
          <dd>{info.counts.somas}</dd>
          <dt>{t.tabs.sessions}</dt>
          <dd>{info.counts.sessions}</dd>
          <dt>{t.openSomas}</dt>
          <dd>{info.counts.openSomas}</dd>
        </dl>
      </div>
      <div class="admin-card">
        <h3>{t.storage}</h3>
        <dl>
          <dt>{t.somasDir}</dt>
          <dd>{formatSize(info.storage.somasBytes)} · {t.files(info.storage.somasFiles)}<div class="admin-faint admin-mono">{info.storage.somasDir}</div></dd>
          <dt>{t.dataDir}</dt>
          <dd>{formatSize(info.storage.dataBytes)}<div class="admin-faint admin-mono">{info.storage.dataDir}</div></dd>
        </dl>
      </div>
      <div class="admin-card">
        <h3>{t.limits}</h3>
        <dl>
          <dt>{t.maxNote}</dt>
          <dd>{formatSize(info.limits.maxNoteBytes)}</dd>
          <dt>{t.maxUpload}</dt>
          <dd>{formatSize(info.limits.maxUploadBytes)}</dd>
        </dl>
      </div>
      <div class="admin-card wide">
        <h3>{t.sso}</h3>
        {#if info.sso}
          <dl>
            <dt>{t.ssoState}</dt>
            <dd>
              <span class="admin-pill" class:accent={info.sso.ready} class:warn={!info.sso.ready}>{info.sso.ready ? t.ssoReady : t.ssoDown}</span>
              {#if info.sso.error}<div class="admin-faint">{info.sso.error}</div>{/if}
            </dd>
            <dt>{t.ssoIssuer}</dt>
            <dd class="admin-mono">{info.sso.issuer}</dd>
            <dt>{t.ssoClient}</dt>
            <dd class="admin-mono">{info.sso.clientId}</dd>
            <dt>{t.ssoRedirect}</dt>
            <dd class="admin-mono">{info.sso.redirectUrl}</dd>
            <dt>{t.ssoLogout}</dt>
            <dd>{info.sso.logout ? t.yes : t.no}</dd>
            <dt>{t.ssoChecked}</dt>
            <dd>{admin.formatDate(info.sso.checkedAt) || t.never}</dd>
            <dt>{t.passwords}</dt>
            <dd>{info.localLogin ? t.passwordsAll : t.passwordsAdmin}</dd>
          </dl>
          <div class="admin-actions">
            <button type="button" class="admin-btn" disabled={checking} onclick={() => void check()}>{t.ssoCheck}</button>
          </div>
        {:else}
          <p class="admin-faint">{t.ssoOff}</p>
        {/if}
      </div>
    </div>

    {#if info.open.length}
      <div class="admin-scroll">
        <table class="admin-table">
          <thead>
            <tr><th>{t.openSomas}</th><th class="num">{t.notes}</th><th class="num">{t.streams}</th><th class="num">{t.editing}</th></tr>
          </thead>
          <tbody>
            {#each info.open as o (o.id)}
              <tr>
                <td>{o.name || o.id}</td>
                <td class="num">{o.notes}</td>
                <td class="num">{o.streams}</td>
                <td class="num">{o.editing}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}

    <div>
      <h2 class="section-title">{t.configuration}</h2>
      <p class="admin-faint">{t.configNote}</p>
    </div>
    <div class="admin-scroll">
      <table class="admin-table">
        <tbody>
          {#each settings as [name, value] (name)}
            <tr>
              <td class="admin-mono">{name}</td>
              <td class="admin-mono" class:admin-faint={!value}>{value || t.notSet}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .section-title {
    margin: var(--space-2) 0 2px;
    font-size: var(--text-md);
    font-weight: 600;
  }
</style>
