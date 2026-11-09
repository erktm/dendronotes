import { describe, expect, it } from 'vitest'
import { capabilities, rank, SHARE_ROLES } from './roles'

describe('roles', () => {
  it('orders roles like the server', () => {
    expect(['viewer', 'writer', 'contributor', 'editor', 'owner'].map((r) => rank(r as never))).toEqual([1, 2, 3, 4, 5])
    expect(rank(null)).toBe(0)
    expect(SHARE_ROLES).not.toContain('owner')
  })

  it('gives each role what it may do', () => {
    expect(capabilities('viewer')).toEqual({ write: false, create: false, organize: false, manage: false })
    expect(capabilities('writer')).toEqual({ write: true, create: false, organize: false, manage: false })
    expect(capabilities('contributor')).toEqual({ write: true, create: true, organize: false, manage: false })
    expect(capabilities('editor')).toEqual({ write: true, create: true, organize: true, manage: false })
    expect(capabilities('owner')).toEqual({ write: true, create: true, organize: true, manage: true })
    expect(capabilities(undefined)).toEqual({ write: false, create: false, organize: false, manage: false })
  })
})
