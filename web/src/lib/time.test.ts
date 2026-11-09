import { describe, expect, it } from 'vitest'
import { ago, stamp } from './time'

describe('ago', () => {
  const now = new Date('2026-11-09T12:00:00Z').getTime()
  it('says how long ago', () => {
    expect(ago('2026-11-09T11:59:30Z', now)).toBe('just now')
    expect(ago('2026-11-09T11:55:00Z', now)).toBe('5 minutes ago')
    expect(ago('2026-11-09T09:00:00Z', now)).toBe('3 hours ago')
    expect(ago('2026-11-08T11:00:00Z', now)).toBe('yesterday')
    expect(ago('2026-11-05T11:00:00Z', now)).toBe('4 days ago')
    expect(ago('2026-10-01T11:00:00Z', now)).toMatch(/^on Oct 1, 2026/)
    expect(ago('nonsense', now)).toBe('')
  })
  it('stamps today with the time only', () => {
    const local = new Date(2026, 10, 9, 10, 24).getTime()
    expect(stamp(new Date(2026, 10, 9, 9, 5).toISOString(), local)).toMatch(/^today, 9:05/)
    expect(stamp(new Date(2026, 10, 1, 9, 5).toISOString(), local)).toMatch(/^Nov 1, 2026/)
  })
})
