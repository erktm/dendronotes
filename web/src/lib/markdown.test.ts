// @vitest-environment jsdom
// DOMPurify is built and tested against jsdom; happy-dom's global window confuses it.
import { describe, expect, it } from 'vitest'
import { renderMarkdown } from './markdown'
import { noteURL } from './router.svelte'
import { setSoma } from './api/base'

setSoma('soma1')

const notes = ['Soil types.md', 'Projects/Plan.md']
const files = ['photo.png', 'Projects/north bed.jpg', 'Projects/report.pdf', 'song.mp3', 'clip.mp4', 'Garden.drawing.svg', ...notes]
const find = (list: string[], want: string) =>
  list.find((n) => n.toLowerCase() === want.toLowerCase() || n.toLowerCase().endsWith(`/${want.toLowerCase()}`)) ?? null
const ctx = {
  noteURL: (path: string) => noteURL('soma1', path),
  resolve: (target: string, markdown: boolean) => find(notes, (markdown ? target : `${target}.md`).replace(/^\.\.\//, '')),
  resolveFile: (target: string) => find(files, target.replace(/^\.\.\//, '')),
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
  '<img src="//evil.example/api/somas/soma1/files/x.png">',
  '<img src="https://evil.example/api/somas/soma1/files/x.png">',
  '<img src="/api/somas/soma1/files/../../x" onerror=alert(1)>',
  '<audio src="javascript:alert(1)" controls></audio>',
  '<video src="data:video/mp4;base64,AAAA"></video>',
  '<img src="/api/soma/files\\..\\x.png">',
  '<img srcset="https://evil.example/x.png 1x" src="/api/somas/soma1/files/photo.png">',
  '![[Garden.drawing.svg|300" onload="alert(1)]]',
  '<span class="drawing-embed" data-drawing="x"><img src="https://evil.example/x.drawing.svg"></span>',
]

describe('renderMarkdown sanitizing', () => {
  for (const payload of XSS) {
    it(`neutralizes ${payload}`, () => {
      const el = dom(payload)
      expect(el.querySelector('script, iframe, object, embed, svg, math, style, form, base, meta, link, button, source')).toBeNull()
      // Media may only load from the soma.
      for (const m of el.querySelectorAll('img, audio, video')) {
        expect(m.getAttribute('src') ?? '').toMatch(/^\/api\/somas\/soma1\/files\/[^\\]*$/)
        expect(m.hasAttribute('srcset')).toBe(false)
      }
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
    expect(soil!.getAttribute('href')).toBe('/s/soma1/n/Soil%20types.md')
    expect(soil!.textContent).toBe('Soil types')
    expect(plan!.textContent).toBe('the plan')
    expect(plan!.getAttribute('href')).toBe('/s/soma1/n/Projects/Plan.md')
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
    expect(a.getAttribute('href')).toBe('/s/soma1/n/Soil%20types.md')
  })

  it('renders task lists with their source line', () => {
    const el = dom('# Tasks\n\n- [x] Done\n- [ ] Open')
    const boxes = [...el.querySelectorAll<HTMLInputElement>('input.task-box')]
    expect(boxes.map((b) => [b.dataset.line, b.checked])).toEqual([
      ['2', true],
      ['3', false],
    ])
  })

  it('gives headings anchors and shows missing images as placeholders', () => {
    const el = dom('## Garden Plan\n\n![bed](north-bed-missing.jpg)')
    expect(el.querySelector('h2')!.id).toBe('garden-plan')
    expect(el.querySelector('img')).toBeNull()
    expect(el.querySelector('.attachment.missing')!.textContent).toContain('bed')
  })

  it('shows images, audio and video from the soma', () => {
    const el = dom('![[photo.png|300]] ![[north bed.jpg|tomatoes]]\n\n![a bed](north%20bed.jpg)\n\n![[song.mp3]] ![[clip.mp4]]')
    const [a, b, c] = [...el.querySelectorAll('img')]
    expect(a!.getAttribute('src')).toBe('/api/somas/soma1/files/photo.png')
    expect(a!.getAttribute('width')).toBe('300')
    expect(b!.getAttribute('src')).toBe('/api/somas/soma1/files/Projects/north%20bed.jpg')
    expect(b!.getAttribute('alt')).toBe('tomatoes')
    expect(c!.getAttribute('alt')).toBe('a bed')
    const audio = el.querySelector('audio')!
    expect(audio.getAttribute('src')).toBe('/api/somas/soma1/files/song.mp3')
    expect(audio.hasAttribute('controls')).toBe(true)
    expect(el.querySelector('video')!.getAttribute('preload')).toBe('metadata')
  })

  it('marks drawings for drawing in place', () => {
    const el = dom('![[Garden.drawing.svg|480]]\n\n![plan](Garden.drawing.svg)')
    const [a, b] = [...el.querySelectorAll<HTMLElement>('.drawing-embed')]
    expect(a!.dataset.drawing).toBe('Garden.drawing.svg')
    const img = a!.querySelector('img')!
    expect(img.getAttribute('src')).toBe('/api/somas/soma1/files/Garden.drawing.svg')
    expect(img.getAttribute('width')).toBe('480')
    expect(img.classList.contains('embed-drawing')).toBe(true)
    expect(b!.querySelector('img')!.getAttribute('alt')).toBe('plan')
    expect(b!.querySelector('img')!.hasAttribute('width')).toBe(false)
  })

  it('links other attachments and keeps external images out', () => {
    const el = dom('![[report.pdf]] [the report](Projects/report.pdf) ![x](https://example.org/x.png) ![[gone.png]]')
    const links = [...el.querySelectorAll('a.file')]
    expect(links.map((a) => a.getAttribute('href'))).toEqual(['/api/somas/soma1/files/Projects/report.pdf', '/api/somas/soma1/files/Projects/report.pdf'])
    expect(links.every((a) => a.getAttribute('target') === '_blank' && a.getAttribute('rel') === 'noopener')).toBe(true)
    expect(el.textContent).toContain('External image not loaded')
    expect(el.textContent).toContain('Missing attachment: gone.png')
    expect(el.querySelector('img')).toBeNull()
  })

  it('strips what media must not do', () => {
    const el = dom('<video src="/api/somas/soma1/files/clip.mp4" autoplay loop poster="https://evil.example/p.png" width="50%" onplay="alert(1)"></video>')
    const v = el.querySelector('video')!
    expect([...v.attributes].map((a) => a.name).sort()).toEqual(['src'])
  })

  it('marks embedded notes for loading', () => {
    const el = dom('![[Plan#Tasks]]\n\n![[Nowhere]]')
    const embed = el.querySelector<HTMLElement>('.embed-note')!
    expect(embed.dataset.embed).toBe('Projects/Plan.md')
    expect(embed.dataset.embedHeading).toBe('Tasks')
    expect(embed.querySelector('.embed-head a.synapse')!.getAttribute('href')).toBe('/s/soma1/n/Projects/Plan.md#tasks')
    expect(el.querySelectorAll('.embed-note')).toHaveLength(1)
    expect(el.querySelector('.embedded-note a.unresolved')).not.toBeNull()
  })
})
