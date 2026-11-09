<script lang="ts">
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import * as api from '../lib/api/tokens'
  import type { APIToken, TokenScope } from '../lib/api/tokens'
  import { errorMessage } from '../lib/errors'
  import { ago, stamp } from '../lib/time'
  import { strings } from '../lib/strings'

  const t = strings.profile

  let list = $state.raw<APIToken[] | null>(null)
  let name = $state('')
  let scope = $state<TokenScope>('read')
  let days = $state(90)
  let created = $state<string | null>(null)
  let busy = $state(false)

  async function load() {
    try {
      list = await api.tokens()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }
  $effect(() => {
    void load()
  })

  async function create(e: SubmitEvent) {
    e.preventDefault()
    if (!name.trim() || busy) return
    busy = true
    try {
      created = (await api.createToken(name.trim(), scope, days)).token
      name = ''
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  async function revoke(tok: APIToken) {
    const ok = await app.ask({ title: t.revokeTitle(tok.name), message: t.revokeText, confirmLabel: t.revoke, danger: true })
    if (!ok) return
    try {
      await api.revokeToken(tok.id)
      app.toast(t.revoked(tok.name), 'info')
      await load()
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  async function copy() {
    if (!created) return
    try {
      await navigator.clipboard.writeText(created)
      app.toast(t.copied, 'info')
    } catch {
      // the field is selectable as well
    }
  }

  const meta = (tok: APIToken) =>
    t.tokenMeta(t.tokenScopes[tok.scope] ?? tok.scope, ago(tok.createdAt), tok.expiresAt ? t.tokenExpires(stamp(tok.expiresAt)) : t.tokenNeverExpires)
</script>

<section class="section tokens">
  <h3>{t.tokensTitle}</h3>
  <p class="hint">{t.tokensText}</p>

  {#if created}
    <div class="created" role="status">
      <p>{t.tokenCreated}</p>
      <div class="copyrow">
        <input readonly value={created} aria-label={t.tokensTitle} onfocus={(e) => e.currentTarget.select()} />
        <button type="button" class="btn" onclick={() => void copy()}>{t.copy}</button>
      </div>
    </div>
  {/if}

  {#if list && list.length}
    <ul>
      {#each list as tok (tok.id)}
        <li>
          <span class="what">
            <strong>{tok.name}</strong>
            <span class="muted">{meta(tok)}</span>
            <span class="muted">{tok.lastUsedAt ? t.tokenUsed(ago(tok.lastUsedAt), tok.lastUsedIp ?? '') : t.tokenUnused}</span>
          </span>
          <button type="button" class="icon-btn" aria-label={`${t.revoke}: ${tok.name}`} title={t.revoke} onclick={() => void revoke(tok)}>
            <Icon name="trash" size={15} />
          </button>
        </li>
      {/each}
    </ul>
  {:else if list}
    <p class="muted">{t.noTokens}</p>
  {/if}

  <form onsubmit={create}>
    <label for="token-name">{t.tokenName}</label>
    <input id="token-name" bind:value={name} maxlength="64" placeholder={t.tokenNamePlaceholder} autocomplete="off" />
    <div class="pair">
      <div>
        <label for="token-scope">{t.tokenScope}</label>
        <select id="token-scope" bind:value={scope}>
          <option value="read">{t.tokenScopes.read}</option>
          <option value="write">{t.tokenScopes.write}</option>
        </select>
      </div>
      <div>
        <label for="token-days">{t.tokenExpiry}</label>
        <select id="token-days" bind:value={days}>
          {#each t.tokenExpiries as opt (opt.days)}
            <option value={opt.days}>{opt.label}</option>
          {/each}
        </select>
      </div>
    </div>
    <div class="actions">
      <button type="submit" class="primary" disabled={busy || !name.trim()}>{t.createToken}</button>
    </div>
  </form>
</section>

<style>
  .section {
    display: flex;
    flex-direction: column;
    padding: 16px 20px 18px;
    border-top: 1px solid var(--line);
  }
  h3 {
    margin: 0 0 4px;
    font-size: 11.5px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .hint,
  .muted {
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  ul {
    margin: 10px 0 0;
    padding: 0;
    list-style: none;
  }
  li {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 0;
    border-top: 1px solid var(--line);
  }
  .what {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }
  .what strong {
    font-size: var(--text-sm);
  }
  .created {
    margin-top: 10px;
    padding: 10px;
    border: 1px solid color-mix(in srgb, var(--ok) 50%, var(--line));
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--ok) 8%, transparent);
    font-size: var(--text-sm);
  }
  .copyrow {
    display: flex;
    gap: 6px;
    margin-top: 6px;
  }
  .copyrow input {
    flex: 1;
    min-width: 0;
    font-family: var(--font-mono);
    font-size: 12px;
  }
  form {
    display: flex;
    flex-direction: column;
  }
  label {
    margin: 12px 0 6px;
    font-size: var(--text-sm);
    font-weight: 550;
  }
  input,
  select {
    height: 36px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
  }
  .pair {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }
  .pair > div {
    display: flex;
    flex-direction: column;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
  }
  .primary,
  .btn {
    height: 34px;
    padding: 0 12px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
    font-weight: 550;
    cursor: pointer;
  }
  .primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--on-accent);
  }
  .primary:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .icon-btn {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .icon-btn:hover {
    background: var(--bg-hover);
    color: var(--err);
  }
</style>
