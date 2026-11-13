import { describe, expect, it } from 'vitest'
import { markdownWithTitle } from './noteExport'

describe('note export', () => {
  it('puts the title on top', () => {
    expect(markdownWithTitle('Plan', 'Some text\n')).toBe('# Plan\n\nSome text\n')
    expect(markdownWithTitle('Plan', '\n\n  \nText')).toBe('# Plan\n\nText')
    expect(markdownWithTitle('Plan', '﻿Text')).toBe('# Plan\n\nText')
    expect(markdownWithTitle('Empty', '')).toBe('# Empty\n')
  })

  it('does not repeat a title the note starts with', () => {
    expect(markdownWithTitle('Plan', '# Plan\n\nText')).toBe('# Plan\n\nText')
    expect(markdownWithTitle('Plan', '# Other\n\nText')).toBe('# Plan\n\n# Other\n\nText')
  })
})
