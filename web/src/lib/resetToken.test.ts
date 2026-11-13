import { describe, expect, it } from 'vitest'
import { tokenFromHash } from './resetToken'

describe('reset links', () => {
  it('take the token from the fragment', () => {
    const t = 'aB3_-'.repeat(9)
    expect(tokenFromHash('#' + t)).toBe(t)
    expect(tokenFromHash(t)).toBe(t)
  })

  it('ignore anything that is not a token', () => {
    for (const h of ['', '#', '#short', '#with space and more letters', '#<script>alert(1)</script>', '#' + 'a'.repeat(200)]) {
      expect(tokenFromHash(h)).toBe('')
    }
  })
})
