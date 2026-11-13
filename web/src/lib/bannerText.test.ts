// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { bannerHTML } from './bannerText'

describe('banner texts', () => {
  it('render short Markdown', () => {
    expect(bannerHTML('**Update** tonight, _maybe_ `v0.2`\nsecond line')).toBe(
      '<strong>Update</strong> tonight, <em>maybe</em> <code>v0.2</code><br>\nsecond line',
    )
    expect(bannerHTML('~~old~~ new')).toBe('<s>old</s> new')
  })

  it('open external links safely and keep local ones', () => {
    expect(bannerHTML('[notes](https://notes.example.org/s/abc)')).toBe(
      '<a href="https://notes.example.org/s/abc" target="_blank" rel="noopener noreferrer nofollow">notes</a>',
    )
    expect(bannerHTML('see https://example.org')).toContain('<a href="https://example.org" target="_blank"')
    expect(bannerHTML('[admin](/admin)')).toBe('<a href="/admin">admin</a>')
  })

  it('never let markup or scripts through', () => {
    const corpus = [
      '<script>alert(1)</script>',
      '<img src=x onerror=alert(1)>',
      '[x](javascript:alert(1))',
      '[x](JaVaScRiPt:alert(1))',
      '[x](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)',
      '[x](//evil.example.org)',
      '[x](vbscript:msgbox(1))',
      '<a href="https://example.org" onclick="alert(1)">x</a>',
      '<svg onload=alert(1)>',
      '<iframe src="https://example.org"></iframe>',
      '# heading\n> quote\n- list',
      '![image](https://example.org/x.png)',
    ]
    for (const src of corpus) {
      const html = bannerHTML(src)
      const div = document.createElement('div')
      div.innerHTML = html
      for (const el of div.querySelectorAll('*')) {
        expect(['STRONG', 'EM', 'S', 'CODE', 'A', 'BR']).toContain(el.tagName)
        for (const a of el.attributes) expect(['href', 'target', 'rel']).toContain(a.name)
        const href = el.getAttribute('href')
        if (href !== null) expect(href).toMatch(/^(https?:\/\/|\/(?!\/)|#)/i)
      }
      // Markup in the text stays text.
      expect(html).not.toMatch(/<(script|img|svg|iframe|h1|blockquote|ul)/i)
    }
  })
})
