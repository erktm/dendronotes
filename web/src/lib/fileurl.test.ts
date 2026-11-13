import { describe, expect, it } from 'vitest'
import { isFileURL } from './files'

describe('file URLs', () => {
  it('are a soma’s files or those of a public link, nothing else', () => {
    const token = 'x'.repeat(43)
    expect(isFileURL('/api/somas/abc123/files/Beds/a.png')).toBe(true)
    expect(isFileURL(`/api/public/${token}/files/Beds/a.png`)).toBe(true)
    for (const bad of [
      '/api/public/short/files/a.png',
      `/api/public/${token}/a.png`,
      `/api/public/${token}/x/files/a.png`,
      'https://evil.example/api/somas/abc/files/a.png',
      '//evil.example/api/somas/abc/files/a.png',
      '/api/somas/ABC/files/a.png',
      '/api/somas/abc/files/a\\b.png',
    ]) {
      expect(isFileURL(bad), bad).toBe(false)
    }
  })
})
