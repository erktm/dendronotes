// Banner texts: short Markdown the admins write (bold, italic, code, links,
// line breaks). The only way banner text becomes HTML: markdown-it renders
// inline without raw HTML, DOMPurify keeps a handful of tags, and links go
// only to http(s) or to paths of this server.

import DOMPurify from 'dompurify'
import MarkdownIt from 'markdown-it'
import type { MarkdownIt as MarkdownItInstance } from 'markdown-it'

let md: MarkdownItInstance | null = null
let purifier: typeof DOMPurify | null = null

const EXTERNAL = /^https?:\/\//i

function renderer(): MarkdownItInstance {
  if (!md) {
    md = new MarkdownIt('zero', { html: false, linkify: true, breaks: true })
    md.enable(['emphasis', 'strikethrough', 'backticks', 'link', 'autolink', 'linkify', 'newline', 'escape', 'entity'])
  }
  return md
}

function purify(): typeof DOMPurify {
  if (purifier) return purifier
  const p = DOMPurify(window)
  p.addHook('afterSanitizeAttributes', (node) => {
    if (node.nodeName !== 'A') return
    const el = node as Element
    if (EXTERNAL.test(el.getAttribute('href') ?? '')) {
      el.setAttribute('target', '_blank')
      el.setAttribute('rel', 'noopener noreferrer nofollow')
    } else {
      el.removeAttribute('target')
      el.removeAttribute('rel')
    }
  })
  purifier = p
  return p
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`)
}

/** Sanitized HTML for a banner text. */
export function bannerHTML(src: string): string {
  const p = purify()
  if (!p.isSupported) return escapeHtml(src)
  return p.sanitize(renderer().renderInline(src), {
    ALLOWED_TAGS: ['strong', 'em', 's', 'code', 'a', 'br'],
    ALLOWED_ATTR: ['href', 'target', 'rel'],
    // http(s) and paths on this server only (no protocol-relative //host).
    ALLOWED_URI_REGEXP: /^(?:https?:\/\/|\/(?!\/)|#)/i,
  })
}
