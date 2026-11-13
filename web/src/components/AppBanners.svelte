<script lang="ts">
  import BannerList from './BannerList.svelte'
  import * as api from '../lib/api/soma'
  import type { ShownBanner } from '../lib/api/soma'
  import { app } from '../lib/state/app.svelte'
  import { live } from '../lib/state/live.svelte'
  import { errorMessage } from '../lib/errors'

  // The banners of the signed-in account, above the whole app. They are
  // loaded again when the admins change them, when the tab comes back and
  // every five minutes (for start and end times).
  const EVERY = 5 * 60 * 1000

  let banners = $state.raw<ShownBanner[]>([])

  async function load() {
    try {
      banners = await api.myBanners()
    } catch {
      // keep what is shown; the next round tries again
    }
  }

  $effect(() => {
    void live.bannersVersion
    void load()
  })

  $effect(() => {
    const timer = setInterval(() => void load(), EVERY)
    const onvisible = () => {
      if (document.visibilityState === 'visible') void load()
    }
    document.addEventListener('visibilitychange', onvisible)
    return () => {
      clearInterval(timer)
      document.removeEventListener('visibilitychange', onvisible)
    }
  })

  async function dismiss(id: number) {
    banners = banners.filter((b) => b.id !== id)
    try {
      await api.dismissBanner(id)
    } catch (err) {
      app.toast(errorMessage(err))
      void load()
    }
  }
</script>

<BannerList {banners} ondismiss={(id) => void dismiss(id)} />
