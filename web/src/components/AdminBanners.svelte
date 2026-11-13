<script lang="ts">
  import BannerList from './BannerList.svelte'
  import Icon from './Icon.svelte'
  import * as admin from '../lib/api/admin'
  import type { Banner, BannerAudience, BannerInput, BannerKind } from '../lib/api/admin'
  import type { Group } from '../lib/api/soma'
  import { closeOnBackdrop } from '../lib/actions'
  import { app } from '../lib/state/app.svelte'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  // Admin → Banners: announcements for the sign-in page and the app.
  const t = strings.admin
  const KINDS: BannerKind[] = ['info', 'warning', 'critical']
  const AUDIENCES: BannerAudience[] = ['everyone', 'admins', 'group']

  let list = $state.raw<Banner[] | null>(null)
  let groups = $state.raw<Group[]>([])

  async function load() {
    try {
      list = await admin.banners()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  $effect(() => {
    void load()
    admin.groups().then(
      (g) => (groups = g.filter((x) => !x.everyone)),
      () => {},
    )
  })

  // --- Times: <input type="datetime-local"> works in local time ------------

  const set = (iso: string) => {
    const d = new Date(iso)
    return !isNaN(d.getTime()) && d.getUTCFullYear() > 1970
  }
  function toLocal(iso: string): string {
    if (!set(iso)) return ''
    const d = new Date(iso)
    const p = (n: number) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
  }
  const fromLocal = (v: string) => (v ? new Date(v).toISOString() : null)

  function status(b: Banner): 'active' | 'scheduled' | 'ended' {
    const now = Date.now()
    if (set(b.endsAt) && new Date(b.endsAt).getTime() <= now) return 'ended'
    if (set(b.startsAt) && new Date(b.startsAt).getTime() > now) return 'scheduled'
    return 'active'
  }

  // --- Dialog ----------------------------------------------------------------

  let dialog: HTMLDialogElement | undefined = $state()
  let editing = $state.raw<Banner | null>(null)
  let busy = $state(false)
  let formError = $state('')
  let form = $state({
    message: '',
    kind: 'info' as BannerKind,
    inApp: true,
    onLogin: false,
    audience: 'everyone' as BannerAudience,
    groupId: 0,
    dismissible: true,
    starts: '',
    ends: '',
  })

  function open(b: Banner | null) {
    editing = b
    formError = ''
    form = {
      message: b?.message ?? '',
      kind: b?.kind ?? 'info',
      inApp: b?.inApp ?? true,
      onLogin: b?.onLogin ?? false,
      audience: b?.audience ?? 'everyone',
      groupId: b?.groupId ?? groups[0]?.id ?? 0,
      dismissible: b?.dismissible ?? true,
      starts: toLocal(b?.startsAt ?? ''),
      ends: toLocal(b?.endsAt ?? ''),
    }
    dialog?.showModal()
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    const input: BannerInput = {
      message: form.message,
      kind: form.kind,
      inApp: form.inApp,
      onLogin: form.audience === 'everyone' && form.onLogin,
      audience: form.audience,
      groupId: form.audience === 'group' ? form.groupId : undefined,
      dismissible: form.dismissible,
      startsAt: fromLocal(form.starts),
      endsAt: fromLocal(form.ends),
    }
    busy = true
    formError = ''
    try {
      if (editing) {
        await admin.updateBanner(editing.id, input)
        app.toast(t.bannerSaved, 'info')
      } else {
        await admin.createBanner(input)
        app.toast(t.bannerCreated, 'info')
      }
      dialog?.close()
      await load()
    } catch (err) {
      formError = errorMessage(err)
    } finally {
      busy = false
    }
  }

  async function remove(b: Banner) {
    const ok = await app.ask({ title: t.deleteBanner, message: t.deleteBannerText, confirmLabel: t.deleteBanner, danger: true })
    if (!ok) return
    try {
      await admin.deleteBanner(b.id)
      app.toast(t.bannerDeleted, 'info')
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  const audienceText = (b: Banner) => (b.audience === 'group' ? (b.groupName ?? t.bannerAudiences.group) : t.bannerAudiences[b.audience])
</script>

<div class="admin-page">
  <div class="admin-toolbar">
    <p class="admin-faint intro">{t.bannersText}</p>
    <button type="button" class="admin-btn primary" onclick={() => open(null)}>
      <Icon name="plus" size={15} />
      {t.newBanner}
    </button>
  </div>

  {#if list === null}
    <p class="admin-empty">{t.loading}</p>
  {:else if !list.length}
    <p class="admin-empty">{t.noBanners}</p>
  {:else}
    <ul class="banner-items">
      {#each list as b (b.id)}
        {@const st = status(b)}
        <li>
          <BannerList banners={[b]} variant="card" />
          <div class="meta">
            <span class="admin-pill" class:accent={st === 'active'} class:warn={st === 'ended'}>{t.bannerStatus[st]}</span>
            <span>{t.bannerWhereList(b.inApp, b.onLogin)}</span>
            <span>{audienceText(b)}</span>
            {#if set(b.startsAt) || set(b.endsAt)}
              <span>{admin.formatDate(b.startsAt) || '…'} – {admin.formatDate(b.endsAt) || '…'}</span>
            {/if}
            {#if !b.dismissible}<span>{t.bannerFixed}</span>{/if}
            <span class="spacer"></span>
            <button type="button" class="admin-btn" onclick={() => open(b)}>{t.edit}</button>
            <button type="button" class="admin-btn danger" onclick={() => void remove(b)}>{t.deleteBanner}</button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<dialog class="admin-dialog wide" bind:this={dialog} use:closeOnBackdrop aria-labelledby="banner-dialog-title">
  <form onsubmit={submit}>
    <h2 id="banner-dialog-title">{editing ? t.editBanner : t.newBanner}</h2>
    <label for="b-text">{t.bannerMessage}</label>
    <textarea id="b-text" class="admin-field area" bind:value={form.message} maxlength="1000" rows="3" required></textarea>
    {#if form.message.trim()}
      <span class="sub">{t.bannerPreview}</span>
      <BannerList banners={[{ id: 0, message: form.message, kind: form.kind, dismissible: form.dismissible }]} variant="card" ondismiss={() => {}} />
    {/if}

    <div class="row">
      <div class="col">
        <label for="b-kind">{t.bannerKind}</label>
        <select id="b-kind" class="admin-field" bind:value={form.kind}>
          {#each KINDS as k (k)}<option value={k}>{t.bannerKinds[k]}</option>{/each}
        </select>
      </div>
      <div class="col">
        <label for="b-aud">{t.bannerAudience}</label>
        <select id="b-aud" class="admin-field" bind:value={form.audience}>
          {#each AUDIENCES as a (a)}
            <option value={a} disabled={a === 'group' && !groups.length}>{t.bannerAudiences[a]}</option>
          {/each}
        </select>
      </div>
    </div>
    {#if form.audience === 'group'}
      <label for="b-group">{t.bannerGroup}</label>
      <select id="b-group" class="admin-field" bind:value={form.groupId}>
        {#each groups as g (g.id)}<option value={g.id}>{g.name}</option>{/each}
      </select>
    {/if}

    <span class="sub">{t.bannerWhere}</span>
    <label class="check"><input type="checkbox" bind:checked={form.inApp} /> {t.bannerInApp}</label>
    <label class="check">
      <input type="checkbox" bind:checked={form.onLogin} disabled={form.audience !== 'everyone'} />
      {t.bannerOnLogin}
    </label>
    {#if form.audience !== 'everyone'}<p class="hint">{t.bannerOnLoginHint}</p>{/if}
    <label class="check"><input type="checkbox" bind:checked={form.dismissible} /> {t.bannerDismissible}</label>
    <p class="hint">{t.bannerDismissibleHint}</p>

    <div class="row">
      <div class="col">
        <label for="b-start">{t.bannerStarts}</label>
        <input id="b-start" class="admin-field" type="datetime-local" bind:value={form.starts} />
      </div>
      <div class="col">
        <label for="b-end">{t.bannerEnds}</label>
        <input id="b-end" class="admin-field" type="datetime-local" bind:value={form.ends} />
      </div>
    </div>
    <p class="hint">{t.bannerTimeHint}</p>

    {#if formError}<p class="error" role="alert">{formError}</p>{/if}
    <div class="admin-actions">
      <button type="button" class="admin-btn" onclick={() => dialog?.close()}>{t.cancel}</button>
      <button type="submit" class="admin-btn primary" disabled={busy || !form.message.trim() || (!form.inApp && !form.onLogin)}>
        {editing ? t.save : t.createBanner}
      </button>
    </div>
  </form>
</dialog>

<style>
  .intro {
    flex: 1;
    min-width: 0;
    margin: 0;
  }
  .banner-items {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .banner-items :global(.banners.card) {
    width: 100%;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    margin-top: var(--space-2);
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .spacer {
    flex: 1;
  }
  .admin-dialog.wide {
    width: min(92vw, 560px);
  }
  .area {
    height: auto;
    padding: var(--space-2) var(--space-3);
    resize: vertical;
    line-height: 1.45;
  }
  .sub {
    margin: 12px 0 6px;
    font-size: var(--text-sm);
    font-weight: 550;
  }
  .row {
    display: flex;
    gap: var(--space-3);
  }
  .col {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }
  .check {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 6px 0 0;
    font-weight: 400;
    cursor: pointer;
  }
  .check input {
    accent-color: var(--accent);
  }
  .hint {
    margin-top: 4px;
    font-size: var(--text-xs);
  }
  .error {
    margin-top: 10px;
    color: var(--err);
  }
</style>
