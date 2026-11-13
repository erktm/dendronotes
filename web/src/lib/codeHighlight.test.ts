// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { highlightCodeBlocks } from './codeHighlight'

function block(lang: string, code: string): HTMLElement {
  const root = document.createElement('div')
  const pre = document.createElement('pre')
  const el = document.createElement('code')
  el.className = `language-${lang}`
  el.textContent = code
  pre.append(el)
  root.append(pre)
  document.body.append(root)
  return root
}

describe('code colors', () => {
  it('mark up known languages and keep the text', async () => {
    const src = 'func main() {\n\treturn "hi" // done\n}'
    const root = block('go', src)
    await highlightCodeBlocks(root)
    const code = root.querySelector('code')!
    expect(code.textContent).toBe(src)
    const of = (cls: string) => [...code.querySelectorAll(`.${cls}`)].map((e) => e.textContent)
    expect(of('ct-keyword')).toEqual(expect.arrayContaining(['func', 'return']))
    expect(of('ct-string')).toContain('"hi"')
    expect(of('ct-comment')).toContain('// done')
  })

  it('know languages by their usual names', async () => {
    for (const [lang, src, cls] of [
      ['ts', 'const x: number = 1', 'ct-keyword'],
      ['python', 'def f():\n    pass', 'ct-keyword'],
      ['sql', 'SELECT 1', 'ct-keyword'],
      ['yaml', 'a: "b"', 'ct-string'],
    ] as const) {
      const root = block(lang, src)
      await highlightCodeBlocks(root)
      expect(root.querySelector(`code .${cls}`), lang).not.toBeNull()
    }
  })

  it('leave unknown languages and markup in the code alone', async () => {
    const root = block('no-such-language', '<b>x</b>')
    await highlightCodeBlocks(root)
    expect(root.querySelector('code')!.children.length).toBe(0)
    const html = block('html', '<script>alert(1)</script>')
    await highlightCodeBlocks(html)
    expect(html.querySelector('script')).toBeNull()
    expect(html.querySelector('code')!.textContent).toBe('<script>alert(1)</script>')
  })
})
