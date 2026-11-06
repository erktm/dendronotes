<script lang="ts">
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { settings, type NoteFont, type ThemeChoice } from '../lib/state/settings.svelte'
  import { strings } from '../lib/strings'

  let dialog: HTMLDialogElement | undefined = $state()

  $effect(() => {
    if (!dialog) return
    if (app.settingsOpen && !dialog.open) dialog.showModal()
    else if (!app.settingsOpen && dialog.open) dialog.close()
  })

  const fonts: { value: NoteFont; label: string }[] = [
    { value: 'sans', label: strings.settings.sans },
    { value: 'serif', label: strings.settings.serif },
  ]
  const themes: { value: ThemeChoice; label: string }[] = [
    { value: 'system', label: strings.settings.system },
    { value: 'light', label: strings.settings.light },
    { value: 'dark', label: strings.settings.dark },
  ]
</script>

<dialog bind:this={dialog} aria-labelledby="settings-title" onclose={() => (app.settingsOpen = false)}>
  <header>
    <h2 id="settings-title">{strings.settings.title}</h2>
    <button type="button" class="icon-btn" aria-label={strings.settings.close} onclick={() => (app.settingsOpen = false)}>
      <Icon name="close" size={18} />
    </button>
  </header>

  <section aria-labelledby="settings-editor">
    <h3 id="settings-editor">{strings.settings.editor}</h3>
    <div class="setting">
      <label for="set-live">{strings.settings.livePreview}<span class="desc">{strings.settings.livePreviewDesc}</span></label>
      <input id="set-live" class="switch" type="checkbox" role="switch" bind:checked={settings.livePreview} />
    </div>
    <div class="setting">
      <label for="set-vim">{strings.settings.vim}<span class="desc">{strings.settings.vimDesc}</span></label>
      <input id="set-vim" class="switch" type="checkbox" role="switch" bind:checked={settings.vim} />
    </div>
    <div class="setting">
      <label for="set-readable">{strings.settings.readable}<span class="desc">{strings.settings.readableDesc}</span></label>
      <input id="set-readable" class="switch" type="checkbox" role="switch" bind:checked={settings.readableWidth} />
    </div>
    <div class="setting">
      <span class="name" id="set-font">{strings.settings.noteFont}</span>
      <div class="segmented" role="group" aria-labelledby="set-font">
        {#each fonts as f (f.value)}
          <button type="button" aria-pressed={settings.noteFont === f.value} onclick={() => (settings.noteFont = f.value)}>{f.label}</button>
        {/each}
      </div>
    </div>
  </section>

  <section aria-labelledby="settings-appearance">
    <h3 id="settings-appearance">{strings.settings.appearance}</h3>
    <div class="setting">
      <span class="name" id="set-theme">{strings.settings.theme}</span>
      <div class="segmented" role="group" aria-labelledby="set-theme">
        {#each themes as t (t.value)}
          <button type="button" aria-pressed={settings.theme === t.value} onclick={() => (settings.theme = t.value)}>{t.label}</button>
        {/each}
      </div>
    </div>
    <div class="setting">
      <label for="set-details">{strings.settings.details}<span class="desc">{strings.settings.detailsDesc}</span></label>
      <input id="set-details" class="switch" type="checkbox" role="switch" bind:checked={settings.showDetails} />
    </div>
  </section>

  <section aria-labelledby="settings-soma">
    <h3 id="settings-soma">{strings.settings.soma}</h3>
    <p class="note">{strings.settings.stored}</p>
  </section>
</dialog>

<style>
  dialog {
    width: min(480px, calc(100vw - 32px));
    padding: 0;
    border: 1px solid var(--line);
    border-radius: 14px;
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow), 0 24px 64px rgb(0 0 0 / 0.2);
  }
  dialog::backdrop {
    background: var(--scrim);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 16px 16px 20px;
    border-bottom: 1px solid var(--line);
  }
  h2 {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .icon-btn {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .icon-btn:hover {
    background: var(--bg-hover);
  }
  section {
    padding: 14px 20px 6px;
  }
  section + section {
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
  .setting {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 10px 0;
  }
  .setting label,
  .name {
    font-size: 14px;
    font-weight: 550;
  }
  .desc {
    display: block;
    margin-top: 2px;
    color: var(--text-muted);
    font-size: 12.5px;
    font-weight: 400;
  }
  .note {
    margin: 4px 0 14px;
    color: var(--text-muted);
    font-size: 13px;
  }
  .switch {
    position: relative;
    flex: none;
    width: 38px;
    height: 22px;
    margin: 0;
    border-radius: 999px;
    background: var(--line-strong);
    appearance: none;
    cursor: pointer;
  }
  .switch::after {
    content: '';
    position: absolute;
    top: 3px;
    left: 3px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: var(--bg-raised);
    box-shadow: 0 1px 2px rgb(0 0 0 / 0.2);
    transition: left 0.12s ease;
  }
  .switch:checked {
    background: var(--accent);
  }
  .switch:checked::after {
    left: 19px;
  }
  .switch:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  .segmented {
    display: inline-flex;
    flex: none;
    padding: 2px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg-sunken);
  }
  .segmented button {
    height: 28px;
    padding: 0 12px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text-muted);
    font-size: 12.5px;
    font-weight: 550;
    cursor: pointer;
  }
  .segmented button[aria-pressed='true'] {
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow);
  }
</style>
