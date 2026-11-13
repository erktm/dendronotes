<script lang="ts">
  import BannerList from './BannerList.svelte'
  import { loginBanners, type ShownBanner } from '../lib/api/soma'

  // Banners on the sign-in page. Nobody is signed in yet, so a closed banner
  // is remembered in this browser only (by id and revision of its text).
  const KEY = 'dendronotes.banners.closed'

  function closed(): string[] {
    try {
      const v: unknown = JSON.parse(localStorage.getItem(KEY) ?? '[]')
      return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
    } catch {
      return []
    }
  }

  let banners = $state.raw<ShownBanner[]>([])

  $effect(() => {
    loginBanners().then(
      (list) => {
        const gone = new Set(closed())
        banners = list.filter((b) => !gone.has(`${b.id}:${b.revision}`))
      },
      () => {},
    )
  })

  function dismiss(id: number) {
    const b = banners.find((x) => x.id === id)
    banners = banners.filter((x) => x.id !== id)
    if (!b) return
    try {
      // Only the newest few, so the list never grows without end.
      localStorage.setItem(KEY, JSON.stringify([...closed(), `${b.id}:${b.revision}`].slice(-50)))
    } catch {
      // private window or blocked storage: closed for this visit only
    }
  }
</script>

<BannerList {banners} ondismiss={dismiss} variant="card" />
