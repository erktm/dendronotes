// @vitest-environment jsdom
// DOMPurify is built and tested against jsdom; happy-dom's global window confuses it.
import { describe, expect, it } from 'vitest'
import { renderMarkdown } from './markdown'

const notes = ['Soil types.md', 'Projects/Plan.md']
const ctx = {
  resolve: (target: string, markdown: boolean) => {
    const want = (markdown ? target : `${target}.md`).replace(/^\.\.\//, '').toLowerCase()
    return notes.find((n) => n.toLowerCase() === want || n.toLowerCase().endsWith(`/${want}`)) ?? null
  },
}

function dom(src: string): HTMLElement {
  const el = document.createElement('div')
  el.innerHTML = renderMarkdown(src, ctx)
  return el
}

// Note content is untrusted: everything here must come out harmless.
const XSS = [
  '<script>alert(1)</script>',
  '<img src=x onerror=alert(1)>',
  '[click](javascript:alert(1))',
  '[click](JaVaScRiPt:alert(1))',
  '<a href="javascript:alert(1)">x</a>',
  '<a href="data:text/html,<script>alert(1)</script>">x</a>',
  '<svg onload=alert(1)><circle /></svg>',
  '<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>',
  '<iframe src="https://evil.example"></iframe>',
  '<object data="https://evil.example/x.swf"></object>',
  '<embed src="https://evil.example/x">',
  '<div style="background:url(https://evil.example/track)">styled</div>',
  '<a href="https://example.org" onclick="alert(1)">x</a>',
  '<details open ontoggle=alert(1)>x</details>',
  '<input type="text" autofocus onfocus=alert(1)>',
  '<form action="https://evil.example"><button>send</button></form>',
  '<base href="https://evil.example/">',
  '<meta http-equiv="refresh" content="0;url=https://evil.example">',
  '<link rel="stylesheet" href="https://evil.example/x.css">',
  '<style>body{display:none}</style>',
  '![pixel](https://tracker.example/pixel.png)',
  '[[x" onmouseover="alert(1)]]',
  '[[<img src=x onerror=alert(1)>]]',
  '<video src=x onerror=alert(1)></video>',
]

describe('renderMarkdown sanitizing', () => {
  for (const payload of XSS) {
    it(`neutralizes ${payload}`, () => {
      const el = dom(payload)
      expect(el.querySelector('script, iframe, object, embed, svg, math, img, video, style, form, base, meta, link, button')).toBeNull()
      for (const node of el.querySelectorAll('*')) {
        for (const attr of node.attributes) {
          expect(attr.name.startsWith('on'), `${node.tagName} ${attr.name}`).toBe(false)
          expect(attr.name).not.toBe('style')
        }
      }
      for (const a of el.querySelectorAll('a[href]')) {
        expect(a.getAttribute('href')!.trim().toLowerCase()).not.toMatch(/^(javascript|data|vbscript):/)
      }
      for (const input of el.querySelectorAll('input')) {
        expect(input.getAttribute('type')).toBe('checkbox')
      }
    })
  }
})

describe('renderMarkdown features', () => {
  it('renders synapses with in-app links', () => {
    const el = dom('See [[Soil types]], [[Plan|the plan]] and [[Compost tea]].')
    const [soil, plan, compost] = [...el.querySelectorAll('a.synapse')]
    expect(soil!.getAttribute('href')).toBe('/n/Soil%20types.md')
    expect(soil!.textContent).toBe('Soil types')
    expect(plan!.textContent).toBe('the plan')
    expect(plan!.getAttribute('href')).toBe('/n/Projects/Plan.md')
    expect(compost!.classList.contains('unresolved')).toBe(true)
    expect(compost!.getAttribute('data-synapse')).toBe('Compost tea')
  })

  it('opens external links in a new tab without referrer', () => {
    const a = dom('[docs](https://example.org)').querySelector('a')!
    expect(a.getAttribute('target')).toBe('_blank')
    expect(a.getAttribute('rel')).toContain('noopener')
    expect(a.getAttribute('rel')).toContain('noreferrer')
  })

  it('turns Markdown links to notes into synapses', () => {
    const a = dom('[soil](Soil%20types.md)').querySelector('a')!
    expect(a.classList.contains('synapse')).toBe(true)
    expect(a.getAttribute('href')).toBe('/n/Soil%20types.md')
  })

  it('renders task lists with their source line', () => {
    const el = dom('# Tasks\n\n- [x] Done\n- [ ] Open')
    const boxes = [...el.querySelectorAll<HTMLInputElement>('input.task-box')]
    expect(boxes.map((b) => [b.dataset.line, b.checked])).toEqual([
      ['2', true],
      ['3', false],
    ])
  })

  it('gives headings anchors and does not load images', () => {
    const el = dom('## Garden Plan\n\n![bed](north-bed.jpg)')
    expect(el.querySelector('h2')!.id).toBe('garden-plan')
    expect(el.querySelector('img')).toBeNull()
    expect(el.querySelector('.attachment')!.textContent).toContain('bed')
  })
})
