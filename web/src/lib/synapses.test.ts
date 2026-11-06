import { describe, expect, it } from 'vitest'
import { isNoteTarget, maskCode, newNotePath, normalizePath, parseSynapses, resolveSynapse, synapseLabel } from './synapses'

const targets = (src: string) => parseSynapses(src).map((s) => s.target)

describe('parseSynapses', () => {
  it('finds wiki links with alias and heading', () => {
    const [a, b, c] = parseSynapses('See [[Soil types]], [[Garden log 2025|last year]] and [[Plan#Tasks]].')
    expect(a).toMatchObject({ target: 'Soil types', embed: false, markdown: false })
    expect(b).toMatchObject({ target: 'Garden log 2025', alias: 'last year' })
    expect(c).toMatchObject({ target: 'Plan', heading: 'Tasks' })
  })

  it('treats embedded notes as synapses, attachments not', () => {
    const refs = parseSynapses('![[Weekly review]] ![[north-bed.jpg]] [[manual.pdf]]')
    expect(refs).toHaveLength(1)
    expect(refs[0]).toMatchObject({ target: 'Weekly review', embed: true })
  })

  it('ignores headings in the same note and code', () => {
    const src = ['[[#Tasks]]', '`[[inline code]]`', '```', '[[fenced]]', '```', '~~~md', '[[tilde]]', '~~~', '[[real]]'].join('\n')
    expect(targets(src)).toEqual(['real'])
  })

  it('counts Markdown links to notes, not external ones', () => {
    const src = '[a](Other.md) [b](./sub/My%20Note.md#Part) [c](https://example.org/x.md) [d](mailto:x@y) [e](#local) [f](image.png)'
    const refs = parseSynapses(src)
    expect(refs.map((r) => r.target)).toEqual(['Other.md', './sub/My Note.md'])
    expect(refs[1]).toMatchObject({ heading: 'Part', markdown: true, alias: 'b' })
  })

  it('keeps note names that only look like an extension', () => {
    expect(isNoteTarget('Meeting 12.5')).toBe(true)
    expect(isNoteTarget('photo.PNG')).toBe(false)
    expect(targets('[[Meeting 12.5]]')).toEqual(['Meeting 12.5'])
  })

  it('keeps offsets when masking code', () => {
    const src = 'a `[[x]]` b\n```\n[[y]]\n```'
    expect(maskCode(src)).toHaveLength(src.length)
    expect(maskCode(src)).not.toContain('[[')
  })
})

describe('resolveSynapse', () => {
  const notes = ['Soil types.md', 'Projects/Plan.md', 'Archive/Plan.md', 'Archive/Old/Plan.md', 'Journal/Garden log 2025.md', 'Projects/Sub/Deep.md', 'Café.md']

  it('matches by name, case-insensitively', () => {
    expect(resolveSynapse({ target: 'soil TYPES', markdown: false }, 'Projects/Plan.md', notes)).toBe('Soil types.md')
    expect(resolveSynapse({ target: 'Garden log 2025', markdown: false }, 'x.md', notes)).toBe('Journal/Garden log 2025.md')
    expect(resolveSynapse({ target: 'Café', markdown: false }, 'x.md', notes)).toBe('Café.md')
  })

  it('prefers the same folder, then the shortest path', () => {
    expect(resolveSynapse({ target: 'Plan', markdown: false }, 'Archive/Old/Notes.md', notes)).toBe('Archive/Old/Plan.md')
    expect(resolveSynapse({ target: 'Plan', markdown: false }, 'Journal/x.md', notes)).toBe('Archive/Plan.md')
  })

  it('matches paths and path suffixes', () => {
    expect(resolveSynapse({ target: 'Projects/Plan', markdown: false }, 'x.md', notes)).toBe('Projects/Plan.md')
    expect(resolveSynapse({ target: 'Sub/Deep', markdown: false }, 'x.md', notes)).toBe('Projects/Sub/Deep.md')
  })

  it('resolves Markdown links relative to the note', () => {
    expect(resolveSynapse({ target: '../Soil types.md', markdown: true }, 'Projects/Plan.md', notes)).toBe('Soil types.md')
    expect(resolveSynapse({ target: 'Sub/Deep.md', markdown: true }, 'Projects/Plan.md', notes)).toBe('Projects/Sub/Deep.md')
    expect(resolveSynapse({ target: '../../../etc/passwd.md', markdown: true }, 'Projects/Plan.md', notes)).toBeNull()
  })

  it('returns null for unresolved synapses', () => {
    expect(resolveSynapse({ target: 'Compost tea', markdown: false }, 'x.md', notes)).toBeNull()
  })
})

describe('helpers', () => {
  it('normalizes paths and refuses leaving the soma', () => {
    expect(normalizePath('a/./b/../c.md')).toBe('a/c.md')
    expect(normalizePath('../x.md')).toBeNull()
  })

  it('creates new notes next to the current note', () => {
    expect(newNotePath('Compost tea', 'Projects/Plan.md')).toBe('Projects/Compost tea.md')
    expect(newNotePath('Garden/Compost', 'Projects/Plan.md')).toBe('Garden/Compost.md')
    expect(newNotePath('Idea', 'Top.md')).toBe('Idea.md')
  })

  it('labels synapses', () => {
    expect(synapseLabel({ target: 'Projects/Plan.md' })).toBe('Plan')
    expect(synapseLabel({ target: 'Plan', heading: 'Tasks' })).toBe('Plan › Tasks')
    expect(synapseLabel({ target: 'Plan', alias: 'the plan' })).toBe('the plan')
  })
})
