import { describe, expect, it } from 'vitest'
import { EditorSelection, EditorState } from '@codemirror/state'
import { type InsertKind, markdownInsert } from './markdownInsert'

/** The text after an insert; « and » mark the selection before and after. */
function run(doc: string, kind: InsertKind): string {
  const from = doc.indexOf('«')
  const to = doc.indexOf('»') - 1
  const clean = doc.replace('«', '').replace('»', '')
  const state = EditorState.create({ doc: clean, selection: EditorSelection.single(from, to) })
  const next = state.update(markdownInsert(state, kind)).state
  const sel = next.selection.main
  const text = next.doc.toString()
  return text.slice(0, sel.from) + '«' + text.slice(sel.from, sel.to) + '»' + text.slice(sel.to)
}

describe('inserting Markdown', () => {
  it('sets line marks, replacing others, and takes them off again', () => {
    expect(run('Gar«»den', 'h2')).toBe('## Gar«»den')
    expect(run('## Gar«»den', 'h1')).toBe('# Gar«»den')
    expect(run('# Gar«»den', 'h1')).toBe('Gar«»den')
    expect(run('«one\ntwo»', 'numbered')).toBe('1. «one\n2. two»')
    expect(run('«1. one\n2. two»', 'numbered')).toBe('«one\ntwo»')
    expect(run('  - [x] do«»ne', 'task')).toBe('  - [ ] do«»ne')
    expect(run('- it«»em', 'quote')).toBe('> it«»em')
  })

  it('wraps the selection, or leaves the cursor between the marks', () => {
    expect(run('a «word» here', 'bold')).toBe('a **«word»** here')
    expect(run('a **«word»** here', 'bold')).toBe('a «word» here')
    expect(run('a «»', 'code')).toBe('a `«»`')
    expect(run('see «»', 'synapse')).toBe('see [[«»]]')
  })

  it('fills in links', () => {
    expect(run('«Garden»', 'link')).toBe('[Garden](«url»)')
    expect(run('«»', 'link')).toBe('[«text»](url)')
  })

  it('puts blocks on lines of their own', () => {
    expect(run('Intro«»', 'codeblock')).toBe('Intro\n\n```\n«»\n```\n')
    expect(run('«»', 'rule')).toBe('---\n«»')
    expect(run('Text «» more', 'table')).toBe('Text \n\n| «Column» | Column |\n| --- | --- |\n|  |  |\n\n more')
  })
})
