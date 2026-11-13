// Colors for code blocks: the same token classes in the editor (fenced code
// parsed by the language of its info string) and in the reading view (code
// blocks marked up here after rendering). Languages load on demand from
// @codemirror/language-data; the text goes in only through textContent.

import { LanguageDescription } from '@codemirror/language'
import { highlightTree, tagHighlighter, tags as t } from '@lezer/highlight'

/** Code token → class (styles in styles/editor.css, colors --code-*). */
export const codeHighlighter = tagHighlighter([
  {
    tag: [t.keyword, t.controlKeyword, t.operatorKeyword, t.definitionKeyword, t.moduleKeyword, t.modifier, t.self, t.tagName],
    class: 'ct-keyword',
  },
  { tag: [t.string, t.special(t.string), t.character, t.regexp, t.escape, t.attributeValue], class: 'ct-string' },
  { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], class: 'ct-comment' },
  { tag: [t.number, t.integer, t.float, t.bool, t.atom, t.null, t.unit, t.constant(t.variableName)], class: 'ct-literal' },
  { tag: [t.typeName, t.className, t.namespace, t.standard(t.typeName)], class: 'ct-type' },
  {
    tag: [t.function(t.variableName), t.function(t.propertyName), t.definition(t.function(t.variableName)), t.macroName],
    class: 'ct-function',
  },
  { tag: [t.propertyName, t.attributeName], class: 'ct-property' },
])

let languages: readonly LanguageDescription[] | null = null

/** The language for a code block's info string ("go", "ts", "Dockerfile" …). */
async function languageFor(name: string): Promise<LanguageDescription | null> {
  languages ??= (await import('@codemirror/language-data')).languages
  return LanguageDescription.matchLanguageName(languages, name, true)
}

/** Text split into plain parts and highlighted spans. */
export function highlightedNodes(text: string, desc: LanguageDescription): Node[] {
  const support = desc.support
  if (!support) return [document.createTextNode(text)]
  const tree = support.language.parser.parse(text)
  const out: Node[] = []
  let pos = 0
  highlightTree(tree, codeHighlighter, (from, to, classes) => {
    if (from > pos) out.push(document.createTextNode(text.slice(pos, from)))
    const span = document.createElement('span')
    span.className = classes
    span.textContent = text.slice(from, to)
    out.push(span)
    pos = to
  })
  if (pos < text.length) out.push(document.createTextNode(text.slice(pos)))
  return out
}

/** Colors the code blocks below root (`pre > code.language-…`), once each. */
export async function highlightCodeBlocks(root: ParentNode): Promise<void> {
  const blocks = [...root.querySelectorAll<HTMLElement>('pre > code[class*="language-"]:not([data-highlighted])')]
  for (const code of blocks) {
    code.dataset.highlighted = ''
    const name = [...code.classList].find((c) => c.startsWith('language-'))?.slice('language-'.length)
    if (!name) continue
    const desc = await languageFor(name)
    if (!desc) continue
    if (!desc.support) await desc.load()
    if (!code.isConnected) continue
    code.replaceChildren(...highlightedNodes(code.textContent ?? '', desc))
  }
}
