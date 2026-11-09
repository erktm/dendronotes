import { describe, expect, it } from 'vitest'
import { anchorFromRendered, findAnchor, makeAnchor } from './anchors'

describe('anchors', () => {
  const text = 'Tomatoes go into bed 3.\nBeans go into bed 3 as well.\n'

  it('finds a quote again after edits around it', () => {
    const a = makeAnchor(text, 17, 22) // the first "bed 3"
    expect(a).toEqual({ quote: 'bed 3', prefix: 'Tomatoes go into ', suffix: '.\nBeans go into bed 3 as well.\n' })
    const edited = '# Garden\n\n' + text.replace('Tomatoes', 'Cherry tomatoes')
    expect(findAnchor(edited, a)).toEqual({ from: 34, to: 39 })
    expect(edited.slice(34, 39)).toBe('bed 3')
  })

  it('tells repeated quotes apart by their context', () => {
    const second = makeAnchor(text, 38, 43)
    expect(text.slice(38, 43)).toBe('bed 3')
    expect(findAnchor(text, second)).toEqual({ from: 38, to: 43 })
    expect(findAnchor('Peas go into bed 3 as well.\n' + text, second)).toEqual({ from: 66, to: 71 })
  })

  it('gives up when the quote is gone', () => {
    expect(findAnchor('Tomatoes go into bed 2.', makeAnchor(text, 17, 22))).toBeNull()
    expect(findAnchor(text, { quote: '', prefix: '', suffix: '' })).toBeNull()
  })

  it('places selections from the reading view in the source', () => {
    const source = '# Beds\n\nTomatoes go into **bed 3**.\nBeans go into bed 3 as well.'
    // Selected "bed 3" in the second paragraph line of the rendered text.
    const a = anchorFromRendered(source, 'bed 3', 'Tomatoes go into bed 3.\nBeans go into ', ' as well.')
    expect(findAnchor(source, a)).toEqual({ from: 50, to: 55 })
    // Across formatting the rendered words are not in the source: the context stays.
    const b = anchorFromRendered(source, 'into bed 3.', 'Tomatoes go ', '\nBeans')
    expect(b).toEqual({ quote: 'into bed 3.', prefix: 'Tomatoes go ', suffix: '\nBeans' })
    expect(findAnchor(source, b)).toBeNull()
  })
})
