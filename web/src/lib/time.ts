// Times as people say them: "3 minutes ago", "yesterday", or a date.

const relative = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
const dateTime = new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' })
const time = new Intl.DateTimeFormat('en', { timeStyle: 'short' })

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

/** "just now", "5 minutes ago", "3 hours ago", "yesterday", "4 days ago", else the date. */
export function ago(iso: string, now = Date.now()): string {
  const t = new Date(iso).getTime()
  if (isNaN(t)) return ''
  const diff = Math.max(0, now - t)
  if (diff < MINUTE) return 'just now'
  if (diff < HOUR) return relative.format(-Math.floor(diff / MINUTE), 'minute')
  if (diff < DAY) return relative.format(-Math.floor(diff / HOUR), 'hour')
  if (diff < 7 * DAY) return relative.format(-Math.floor(diff / DAY), 'day')
  return `on ${dateTime.format(t)}`
}

/** A date and time, e.g. "Nov 9, 2026, 10:24 AM" (today: only the time). */
export function stamp(iso: string, now = Date.now()): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return new Date(now).toDateString() === d.toDateString() ? `today, ${time.format(d)}` : dateTime.format(d)
}
