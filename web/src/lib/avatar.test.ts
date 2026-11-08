import { describe, expect, it } from 'vitest'
import { cleanInitials, fallbackAvatar, initials, photoURL } from './avatar'

describe('avatars', () => {
  it('takes initials from names', () => {
    expect(initials('Erik Häger')).toBe('EH')
    expect(initials('anna')).toBe('AN')
    expect(initials('ben.miller@example.org')).toBe('BM')
    expect(initials('Ö')).toBe('Ö')
    expect(initials('  ')).toBe('?')
  })

  it('cleans chosen initials like the server does', () => {
    expect(cleanInitials(' bö ')).toBe('BÖ')
    expect(cleanInitials('a!b')).toBe('AB')
    expect(cleanInitials('abc')).toBe('AB')
    expect(cleanInitials('<>')).toBe('')
  })

  it('links photos by version', () => {
    expect(photoURL(7, { color: 1, pattern: 'plain', initials: '', photo: 0 })).toBeNull()
    expect(photoURL(7, { color: 1, pattern: 'plain', initials: '', photo: 42 })).toBe('/api/users/7/avatar?v=42')
    expect(fallbackAvatar(13)).toEqual({ color: 5, pattern: 'plain', initials: '', photo: 0 })
  })
})
