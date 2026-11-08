<script lang="ts">
  // The profile picture: a design (color, pattern, initials) that is saved
  // on every change, and an optional photo the user places in a circle; the
  // browser crops it to a 256-pixel PNG, the server stores it encoded anew.
  import Avatar from './Avatar.svelte'
  import { app } from '../lib/state/app.svelte'
  import * as api from '../lib/api/soma'
  import type { AvatarPattern, AvatarStyle, User } from '../lib/api/soma'
  import { AVATAR_COLORS, AVATAR_PATTERNS, cleanInitials, cropToPNG, initials } from '../lib/avatar'
  import { errorMessage } from '../lib/errors'
  import { strings } from '../lib/strings'

  let { user }: { user: User } = $props()

  const name = $derived(user.displayName || user.username)
  let busy = $state(false)
  let initialsInput = $state('')
  $effect(() => {
    initialsInput = user.avatar.initials
  })

  async function saveStyle(change: Partial<AvatarStyle>) {
    const a = user.avatar
    const style: AvatarStyle = { color: a.color, pattern: a.pattern, initials: a.initials, ...change }
    try {
      app.setUser(await api.setAvatar(style))
    } catch (err) {
      app.toast(errorMessage(err))
    }
  }

  function commitInitials() {
    const clean = cleanInitials(initialsInput)
    initialsInput = clean
    if (clean !== user.avatar.initials) void saveStyle({ initials: clean })
  }

  // --- Photo ---------------------------------------------------------------

  const MAX_FILE = 20 * 1024 * 1024
  const VIEW = 240 // CSS pixels of the crop area
  let input: HTMLInputElement | undefined = $state()
  let canvas: HTMLCanvasElement | undefined = $state()
  let area: HTMLDivElement | undefined = $state()
  let source = $state<ImageBitmap | null>(null)
  let zoom = $state(1) // 1: the shorter side fills the circle
  let center = $state({ x: 0, y: 0 }) // in image pixels

  async function choose() {
    const file = input?.files?.[0]
    if (input) input.value = ''
    if (!file) return
    if (file.size > MAX_FILE) {
      app.toast(strings.profile.photoTooLarge)
      return
    }
    try {
      source = await createImageBitmap(file)
    } catch {
      app.toast(strings.profile.photoUnreadable)
      return
    }
    zoom = 1
    center = { x: source.width / 2, y: source.height / 2 }
  }

  function cancel() {
    source?.close()
    source = null
  }

  /** The square of the image inside the circle. */
  function crop(img: ImageBitmap, z: number, c: { x: number; y: number }) {
    const side = Math.min(img.width, img.height) / z
    const x = Math.min(Math.max(c.x - side / 2, 0), img.width - side)
    const y = Math.min(Math.max(c.y - side / 2, 0), img.height - side)
    return { x, y, side }
  }

  function clampCenter() {
    if (!source) return
    const r = crop(source, zoom, center)
    center = { x: r.x + r.side / 2, y: r.y + r.side / 2 }
  }

  // Draw the section and darken everything outside the circle.
  $effect(() => {
    const c = canvas
    const img = source
    if (!c || !img) return
    const dpr = window.devicePixelRatio || 1
    c.width = c.height = Math.round(VIEW * dpr)
    const ctx = c.getContext('2d')
    if (!ctx) return
    const r = crop(img, zoom, center)
    ctx.imageSmoothingQuality = 'high'
    ctx.drawImage(img, r.x, r.y, r.side, r.side, 0, 0, c.width, c.height)
    const css = getComputedStyle(c)
    ctx.fillStyle = css.getPropertyValue('--scrim').trim() || 'rgb(0 0 0 / 0.45)'
    ctx.beginPath()
    ctx.rect(0, 0, c.width, c.height)
    ctx.arc(c.width / 2, c.height / 2, c.width / 2 - 1, 0, Math.PI * 2)
    ctx.fill('evenodd')
    ctx.strokeStyle = css.getPropertyValue('--bg-raised').trim() || 'white'
    ctx.lineWidth = dpr
    ctx.beginPath()
    ctx.arc(c.width / 2, c.height / 2, c.width / 2 - 1, 0, Math.PI * 2)
    ctx.stroke()
  })

  let drag: { x: number; y: number } | null = null
  function onpointerdown(e: PointerEvent) {
    drag = { x: e.clientX, y: e.clientY }
    area?.setPointerCapture(e.pointerId)
  }
  function onpointermove(e: PointerEvent) {
    if (!drag || !source) return
    const perPixel = crop(source, zoom, center).side / VIEW
    center = { x: center.x - (e.clientX - drag.x) * perPixel, y: center.y - (e.clientY - drag.y) * perPixel }
    drag = { x: e.clientX, y: e.clientY }
    clampCenter()
  }
  function onpointerup() {
    drag = null
  }
  function onwheel(e: WheelEvent) {
    e.preventDefault()
    zoom = Math.min(4, Math.max(1, zoom * (e.deltaY < 0 ? 1.08 : 1 / 1.08)))
    clampCenter()
  }
  function onkeydown(e: KeyboardEvent) {
    if (!source) return
    const step = crop(source, zoom, center).side / 20
    const moves: Record<string, [number, number]> = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] }
    if (moves[e.key]) {
      const [dx, dy] = moves[e.key]!
      center = { x: center.x + dx, y: center.y + dy }
    } else if (e.key === '+' || e.key === '=') zoom = Math.min(4, zoom * 1.1)
    else if (e.key === '-') zoom = Math.max(1, zoom / 1.1)
    else return
    e.preventDefault()
    clampCenter()
  }

  async function usePhoto() {
    if (!source) return
    busy = true
    try {
      const png = await cropToPNG(source, crop(source, zoom, center), 256)
      app.setUser(await api.uploadAvatarPhoto(png))
      app.toast(strings.profile.photoSaved, 'info')
      cancel()
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  async function removePhoto() {
    busy = true
    try {
      app.setUser(await api.removeAvatarPhoto())
      app.toast(strings.profile.photoRemoved, 'info')
    } catch (err) {
      app.toast(errorMessage(err))
    } finally {
      busy = false
    }
  }

  const patternOf = (p: AvatarPattern) => ({ ...user.avatar, pattern: p, photo: 0 })
  const colorOf = (c: number) => ({ ...user.avatar, color: c, photo: 0 })
</script>

<section class="section" aria-labelledby="avatar-title">
  <h3 id="avatar-title">{strings.profile.picture}</h3>

  {#if source}
    <div class="crop">
      <p class="hint">{strings.profile.cropHint}</p>
      <!-- role="application": a custom widget with its own keys (arrows move, +/- zoom). -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
      <div
        bind:this={area}
        class="area"
        tabindex="0"
        role="application"
        aria-label={strings.profile.cropArea}
        {onpointerdown}
        {onpointermove}
        {onpointerup}
        onpointercancel={onpointerup}
        {onwheel}
        {onkeydown}
      >
        <canvas bind:this={canvas} aria-hidden="true"></canvas>
      </div>
      <label class="zoom">
        <span>{strings.profile.zoom}</span>
        <input type="range" min="1" max="4" step="0.01" bind:value={zoom} oninput={clampCenter} />
      </label>
      <div class="actions">
        <button type="button" class="secondary" onclick={cancel} disabled={busy}>{strings.profile.cancel}</button>
        <button type="button" class="primary" onclick={usePhoto} disabled={busy}>{strings.profile.usePhoto}</button>
      </div>
    </div>
  {:else}
    <div class="top">
      <Avatar id={user.id} {name} avatar={user.avatar} size={72} label={strings.profile.pictureOf(name)} />
      <div class="photo-actions">
        <button type="button" class="secondary" onclick={() => input?.click()} disabled={busy}>
          {user.avatar.photo ? strings.profile.changePhoto : strings.profile.uploadPhoto}
        </button>
        {#if user.avatar.photo}
          <button type="button" class="link" onclick={removePhoto} disabled={busy}>{strings.profile.removePhoto}</button>
        {/if}
      </div>
    </div>
    <input bind:this={input} class="file" type="file" accept="image/png,image/jpeg,image/webp,image/gif,image/avif" onchange={choose} />

    <div class="design">
      <p class="hint">{user.avatar.photo ? strings.profile.photoShown : strings.profile.designHint}</p>

      <span class="label" id="avatar-color">{strings.profile.color}</span>
      <div class="choices" role="group" aria-labelledby="avatar-color">
        {#each Array.from({ length: AVATAR_COLORS }, (_, i) => i) as c (c)}
          <button
            type="button"
            class="choice"
            aria-pressed={user.avatar.color === c}
            aria-label={strings.profile.colorN(c + 1)}
            title={strings.profile.colorN(c + 1)}
            onclick={() => saveStyle({ color: c })}
          >
            <Avatar id={user.id} {name} avatar={colorOf(c)} size={30} />
          </button>
        {/each}
      </div>

      <span class="label" id="avatar-pattern">{strings.profile.pattern}</span>
      <div class="choices" role="group" aria-labelledby="avatar-pattern">
        {#each AVATAR_PATTERNS as p (p)}
          <button
            type="button"
            class="choice"
            aria-pressed={user.avatar.pattern === p}
            aria-label={strings.profile.patterns[p]}
            title={strings.profile.patterns[p]}
            onclick={() => saveStyle({ pattern: p })}
          >
            <Avatar id={user.id} {name} avatar={patternOf(p)} size={40} />
          </button>
        {/each}
      </div>

      <label class="label" for="avatar-initials">{strings.profile.initials}</label>
      <input
        id="avatar-initials"
        class="initials"
        bind:value={initialsInput}
        maxlength="2"
        placeholder={initials(name)}
        aria-describedby="avatar-initials-hint"
        onchange={commitInitials}
      />
      <span class="hint" id="avatar-initials-hint">{strings.profile.initialsHint}</span>
    </div>
  {/if}
</section>

<style>
  .section {
    display: flex;
    flex-direction: column;
    padding: 16px 20px 18px;
    border-bottom: 1px solid var(--line);
  }
  h3 {
    margin: 0 0 10px;
    font-size: 11.5px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
  }
  .top {
    display: flex;
    align-items: center;
    gap: 16px;
  }
  .photo-actions {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
  }
  .file {
    display: none;
  }
  .design {
    display: flex;
    flex-direction: column;
    margin-top: 12px;
  }
  .hint {
    margin: 0;
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  .label {
    margin: 12px 0 6px;
    font-size: var(--text-sm);
    font-weight: 550;
  }
  .choices {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .choice {
    display: grid;
    place-items: center;
    padding: 2px;
    border: 2px solid transparent;
    border-radius: 50%;
    background: transparent;
    cursor: pointer;
  }
  .choice:hover {
    border-color: var(--line-strong);
  }
  .choice[aria-pressed='true'] {
    border-color: var(--accent);
  }
  .choice:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 1px;
  }
  .initials {
    width: 72px;
    height: 38px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font: inherit;
    text-transform: uppercase;
  }
  .initials:focus-visible,
  .area:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  .initials + .hint {
    margin-top: 4px;
  }
  .crop {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
  }
  .area {
    width: 240px;
    height: 240px;
    overflow: hidden;
    border-radius: 8px;
    cursor: grab;
    touch-action: none;
  }
  canvas {
    display: block;
    width: 100%;
    height: 100%;
  }
  .area:active {
    cursor: grabbing;
  }
  .zoom {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 240px;
    font-size: var(--text-sm);
  }
  .zoom input {
    flex: 1;
    accent-color: var(--accent);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    width: 100%;
  }
  .primary,
  .secondary {
    height: 34px;
    padding: 0 var(--space-4);
    border-radius: var(--radius-sm);
    font: inherit;
    font-size: var(--text-sm);
    font-weight: 600;
    cursor: pointer;
  }
  .primary {
    border: 0;
    background: var(--accent);
    color: var(--on-accent);
  }
  .secondary {
    border: 1px solid var(--line-strong);
    background: var(--bg);
    color: var(--text);
  }
  .secondary:hover {
    background: var(--bg-hover);
  }
  .link {
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--err);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .link:hover {
    text-decoration: underline;
  }
  button:disabled {
    cursor: not-allowed;
    opacity: 0.5;
  }
</style>
