<script lang="ts">
  import type { Avatar } from '../lib/api/soma'
  import { fallbackAvatar, initials, photoURL } from '../lib/avatar'
  import { cssVars } from '../lib/actions'

  let {
    id,
    name,
    avatar,
    size = 28,
    label,
  }: {
    /** Account id (for the photo). */
    id: number
    /** Name the initials come from when none are chosen. */
    name: string
    avatar?: Avatar
    size?: number
    /** Accessible name; without it the picture is decorative. */
    label?: string
  } = $props()

  const a = $derived(avatar ?? fallbackAvatar(id))
  const letters = $derived(a.initials || initials(name))

  // A photo that fails to load (removed meanwhile, no access) falls back to
  // the design; a new version tries again.
  let failed = $state<string | null>(null)
  const photo = $derived.by(() => {
    const url = photoURL(id, a)
    return url && url !== failed ? url : null
  })
</script>

<span
  class="avatar peer-c{a.color}"
  use:cssVars={{ '--size': `${size}px` }}
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
>
  {#if photo}
    <img src={photo} alt="" draggable="false" onerror={() => (failed = photo)} />
  {:else}
    <svg viewBox="0 0 40 40">
      {#if a.pattern === 'dendrite'}
        <path class="lines" d="M5 43V31L-1 25M5 31l7-7M5 36l-6-2M35-3V9l6 6M35 9l-7 7M35 4l6-3" />
      {:else if a.pattern === 'rings'}
        <g class="lines">
          <circle cx="33" cy="33" r="5" />
          <circle cx="33" cy="33" r="10" />
          <circle cx="33" cy="33" r="15" />
          <circle cx="5" cy="6" r="5" />
        </g>
      {:else if a.pattern === 'vesicles'}
        <g class="dots">
          <circle cx="7" cy="8" r="2.2" />
          <circle cx="13" cy="4" r="1.4" />
          <circle cx="31" cy="7" r="1.8" />
          <circle cx="36" cy="14" r="1.2" />
          <circle cx="5" cy="29" r="1.6" />
          <circle cx="10" cy="35" r="2.2" />
          <circle cx="33" cy="33" r="1.6" />
          <circle cx="27" cy="37" r="1.1" />
        </g>
      {/if}
      <text x="20" y="20.8" font-size={[...letters].length > 1 ? 15 : 18}>{letters}</text>
    </svg>
  {/if}
</span>

<style>
  .avatar {
    display: inline-grid;
    flex: none;
    place-items: center;
    width: var(--size);
    height: var(--size);
    overflow: hidden;
    border-radius: 50%;
    background: var(--peer);
    color: var(--on-peer);
    user-select: none;
  }
  img,
  svg {
    display: block;
    width: 100%;
    height: 100%;
  }
  img {
    object-fit: cover;
  }
  .lines {
    fill: none;
    stroke: var(--on-peer);
    stroke-width: 1.6;
    stroke-linecap: round;
    stroke-linejoin: round;
    opacity: 0.32;
  }
  .dots {
    fill: var(--on-peer);
    opacity: 0.35;
  }
  text {
    fill: var(--on-peer);
    font-family: var(--font-ui);
    font-weight: 650;
    letter-spacing: 0.02em;
    text-anchor: middle;
    dominant-baseline: central;
  }
</style>
