// Mentions and replies for the signed-in account (the bell in the top bar).

import * as api from '../api/comments'
import type { Notification } from '../api/comments'

class Notifications {
  list = $state.raw<Notification[]>([])
  unread = $state(0)
  loaded = $state(false)

  async load(): Promise<void> {
    try {
      const r = await api.notifications()
      this.list = r.notifications
      this.unread = r.unread
      this.loaded = true
    } catch {
      // the bell keeps what it had
    }
  }

  async read(ids?: number[]): Promise<void> {
    if (ids && ids.length === 0) return
    this.list = this.list.map((n) => (!ids || ids.includes(n.id) ? { ...n, read: true } : n))
    try {
      this.unread = (await api.readNotifications(ids)).unread
    } catch {
      await this.load()
    }
  }
}

export const notifications = new Notifications()
