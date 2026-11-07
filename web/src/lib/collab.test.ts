import { describe, expect, it } from 'vitest'
import { ChangeSet, Text } from '@codemirror/state'
import catalog from './testdata/changesets.json'

// The same catalog is applied by the server (internal/collab); both must
// agree with CodeMirror.
describe('change set catalog', () => {
  it('matches CodeMirror', () => {
    expect(catalog.cases.length).toBeGreaterThan(50)
    for (const c of catalog.cases) {
      const result = ChangeSet.fromJSON(c.changes).apply(Text.of(c.doc.split('\n'))).toString()
      expect(result, c.name).toBe(c.result)
    }
  })

  it('has invalid cases that CodeMirror refuses too (unless marked strict)', () => {
    for (const c of catalog.invalid) {
      if ('strict' in c && c.strict) continue
      expect(() => ChangeSet.fromJSON(c.changes).apply(Text.of(c.doc.split('\n'))), c.name).toThrow()
    }
  })
})
