// Reading view: Markdown → sanitized HTML. markdown-it renders, DOMPurify
// removes everything that could execute (scripts, event handlers,
// javascript: URLs, style attributes, frames, forms, SVG/MathML). Images,
// audio and video are kept only when they come from the soma
// (/api/soma/files/…); everything else that would load is removed.
// Note content is untrusted: it may come from any synced source.

import MarkdownIt from 'markdown-it'
import type { MarkdownIt as MarkdownItInstance, StateCore, StateInline, Token } from 'markdown-it'
import DOMPurify from 'dompurify'
import { basename, encodePath, isNote } from './paths'
import { fileKind, fileURL, isFileURL } from './files'
import { isNoteTarget, splitWikiTarget, synapseLabel } from './synapses'
import { slugger } from './outline'
import { strings } from './strings'

export interface RenderContext {
  /** Resolves a synapse target to a soma path (null: unresolved). */
  resolve: (target: string, markdown: boolean) => string | null
  /** Resolves an attachment to a soma path (null: missing). */
  resolveFile: (target: string, markdown: boolean) => string | null
}

const SCHEME = /^[a-z][a-z0-9+.-]*:/i
const EXTERNAL = /^(https?:|mailto:)/i

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!)
}

const noteHref = (path: string, heading?: string) =>
  `/n/${encodePath(path)}${heading ? `#${slugger()(heading)}` : ''}`

/** "300" or "300x200" after the pipe of an embedded image: its size. */
function parseSize(alias: string | undefined): { w: number; h?: number } | null {
  const m = alias ? /^(\d{1,5})(?:x(\d{1,5}))?$/.exec(alias.trim()) : null
  return m ? { w: Number(m[1]), h: m[2] ? Number(m[2]) : undefined } : null
}

function fileLink(path: string, label: string): string {
  return `<a class="file" href="${escapeHtml(fileURL(path))}" target="_blank" rel="noopener" title="${escapeHtml(strings.reading.openFile(basename(path)))}">${escapeHtml(label)}</a>`
}

/** An attachment shown in place: image, audio or video player, or a link. */
function fileEmbed(path: string, label: string, size: { w: number; h?: number } | null): string {
  const src = escapeHtml(fileURL(path))
  const dims = size ? ` width="${size.w}"${size.h ? ` height="${size.h}"` : ''}` : ''
  switch (fileKind(path)) {
    case 'image':
      return `<img class="embed-image" src="${src}" alt="${escapeHtml(label)}" loading="lazy"${dims}>`
    case 'audio':
      return `<audio class="embed-audio" src="${src}" controls preload="metadata" title="${escapeHtml(label)}"></audio>`
    case 'video':
      return `<video class="embed-video" src="${src}" controls preload="metadata" title="${escapeHtml(label)}"${dims}></video>`
    default:
      return fileLink(path, label)
  }
}

const missing = (name: string) => `<span class="attachment missing">${escapeHtml(strings.reading.missingAttachment(name))}</span>`

function synapsePlugin(md: MarkdownItInstance): void {
  md.inline.ruler.before('link', 'synapse', (state: StateInline, silent: boolean) => {
    let pos = state.pos
    const embed = state.src.charCodeAt(pos) === 0x21 /* ! */
    if (embed) pos++
    if (state.src.charCodeAt(pos) !== 0x5b || state.src.charCodeAt(pos + 1) !== 0x5b) return false
    const end = state.src.indexOf(']]', pos + 2)
    if (end < 0) return false
    const inner = state.src.slice(pos + 2, end)
    if (!inner.trim() || /[[\]\n]/.test(inner)) return false
    if (!silent) {
      const token = state.push('synapse', '', 0)
      token.meta = { inner, embed }
    }
    state.pos = end + 2
    return true
  })

  md.renderer.rules.synapse = (tokens, idx, _options, env) => {
    const ctx = (env as { ctx: RenderContext }).ctx
    const { inner, embed } = tokens[idx]!.meta as { inner: string; embed: boolean }
    const { target, heading, alias } = splitWikiTarget(inner)
    if (!target) {
      // [[#Heading]]: a jump within this note, not a synapse.
      const text = escapeHtml(alias ?? heading ?? '')
      return `<a class="local" href="#${escapeHtml(slugger()(heading ?? ''))}">${text}</a>`
    }
    if (!isNoteTarget(target)) {
      const path = ctx.resolveFile(target, false)
      if (!path) return missing(alias ?? target)
      const size = parseSize(alias)
      const label = size || !alias ? basename(target) : alias
      return embed ? fileEmbed(path, label, size) : fileLink(path, label)
    }
    const resolved = ctx.resolve(target, false)
    const label = escapeHtml(synapseLabel({ target, heading, alias }))
    const cls = resolved ? 'synapse' : 'synapse unresolved'
    const href = resolved ? noteHref(resolved, heading) : '#'
    const link = `<a class="${cls}" href="${escapeHtml(href)}" data-synapse="${escapeHtml(target)}">${label}</a>`
    if (!embed) return link
    if (!resolved) return `<span class="embedded-note">${escapeHtml(strings.reading.embeddedNote)} ${link}</span>`
    // Filled in by the reading view (it loads the note and renders it here).
    const headingAttr = heading ? ` data-embed-heading="${escapeHtml(heading)}"` : ''
    return (
      `<span class="embed-note" data-embed="${escapeHtml(resolved)}"${headingAttr}>` +
      `<span class="embed-head">${link}</span>` +
      `<span class="embed-body"><span class="embed-status">${escapeHtml(strings.reading.embedLoading)}</span></span></span>`
    )
  }
}

function linkPlugin(md: MarkdownItInstance): void {
  const defaultOpen = md.renderer.rules.link_open ?? ((t, i, o, _e, self) => self.renderToken(t, i, o))
  md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
    const ctx = (env as { ctx: RenderContext }).ctx
    const token = tokens[idx]!
    const href = String(token.attrGet('href') ?? '')
    if (EXTERNAL.test(href)) {
      token.attrSet('target', '_blank')
      token.attrSet('rel', 'noopener noreferrer nofollow')
      token.attrJoin('class', 'ext')
    } else if (!SCHEME.test(href) && !href.startsWith('#')) {
      // A Markdown link to another note is a synapse.
      const hash = href.indexOf('#')
      let path = hash >= 0 ? href.slice(0, hash) : href
      try {
        path = decodeURIComponent(path)
      } catch {
        // keep as written
      }
      if (isNote(path)) {
        const resolved = ctx.resolve(path, true)
        token.attrSet('href', resolved ? noteHref(resolved) : '#')
        token.attrSet('data-synapse', path)
        token.attrSet('data-markdown', 'true')
        token.attrJoin('class', resolved ? 'synapse' : 'synapse unresolved')
      } else if (path) {
        // A link to an attachment opens the file.
        const resolved = ctx.resolveFile(path, true)
        token.attrSet('href', resolved ? fileURL(resolved) : '#')
        token.attrJoin('class', resolved ? 'file' : 'file missing')
        if (resolved) {
          token.attrSet('target', '_blank')
          token.attrSet('rel', 'noopener')
        }
      }
    }
    return defaultOpen(tokens, idx, options, env, self)
  }

  // Images: external ones would break the air gap (and the CSP blocks them);
  // files from the soma are shown (images, audio, video) or linked.
  md.renderer.rules.image = (tokens, idx, _options, env) => {
    const ctx = (env as { ctx: RenderContext }).ctx
    const token = tokens[idx]!
    const src = String(token.attrGet('src') ?? '')
    const alt = token.content
    if (SCHEME.test(src) || src.startsWith('//')) {
      return `<span class="attachment">${escapeHtml(strings.reading.externalImage(alt || src))}</span>`
    }
    let target = src.split('#')[0]!
    try {
      target = decodeURIComponent(target)
    } catch {
      // keep as written
    }
    const path = target ? ctx.resolveFile(target, true) : null
    return path ? fileEmbed(path, alt || basename(path), null) : missing(alt || target)
  }
}

function taskPlugin(md: MarkdownItInstance): void {
  md.core.ruler.after('inline', 'tasks', (state: StateCore) => {
    const t = state.tokens
    for (let i = 2; i < t.length; i++) {
      const inline = t[i]!
      if (inline.type !== 'inline' || t[i - 1]!.type !== 'paragraph_open' || t[i - 2]!.type !== 'list_item_open') continue
      const m = /^\[([ xX])\][ \t]/.exec(inline.content)
      const first = inline.children?.[0]
      if (!m || !first || first.type !== 'text' || !first.content.startsWith(m[0])) continue
      first.content = first.content.slice(m[0].length)
      const done = m[1] !== ' '
      const line = t[i - 2]!.map?.[0] ?? -1
      const box = new state.Token('html_inline', '', 0)
      box.content = `<input type="checkbox" class="task-box" data-line="${line}"${done ? ' checked' : ''}> `
      inline.children!.unshift(box)
      t[i - 2]!.attrJoin('class', done ? 'task done' : 'task')
    }
  })
}

function headingIdPlugin(md: MarkdownItInstance): void {
  md.core.ruler.push('heading_ids', (state: StateCore) => {
    const slug = slugger()
    state.tokens.forEach((token: Token, i: number) => {
      if (token.type === 'heading_open') token.attrSet('id', slug(state.tokens[i + 1]?.content ?? ''))
    })
  })
}

/** Top-level blocks carry their source line, so views can scroll to a line. */
function sourceLinePlugin(md: MarkdownItInstance): void {
  md.core.ruler.push('source_lines', (state: StateCore) => {
    for (const token of state.tokens) {
      if (token.level === 0 && token.nesting !== -1 && token.block && token.map) {
        token.attrSet('data-source-line', String(token.map[0]))
      }
    }
  })
}

let md: MarkdownItInstance | null = null

function renderer(): MarkdownItInstance {
  if (!md) {
    md = new MarkdownIt({ html: true, linkify: true, breaks: true, typographer: false })
    md.use(synapsePlugin).use(linkPlugin).use(taskPlugin).use(headingIdPlugin).use(sourceLinePlugin)
  }
  return md
}

const INPUT_ATTRS = new Set(['type', 'checked', 'class', 'data-line'])
const MEDIA_TAGS = new Set(['IMG', 'AUDIO', 'VIDEO'])
const MEDIA_ATTRS = new Set(['src', 'alt', 'title', 'class', 'width', 'height', 'loading', 'controls', 'preload'])

let purifier: typeof DOMPurify | null = null

/** A DOMPurify instance bound explicitly to this window, with our hooks. */
function purify(): typeof DOMPurify {
  if (purifier) return purifier
  const p = DOMPurify(window)
  p.addHook('afterSanitizeAttributes', (node) => {
    if (node.nodeName === 'A') {
      const el = node as Element
      const href = el.getAttribute('href') ?? ''
      if (EXTERNAL.test(href)) {
        el.setAttribute('target', '_blank')
        el.setAttribute('rel', 'noopener noreferrer nofollow')
      } else if (isFileURL(href)) {
        el.setAttribute('target', '_blank')
        el.setAttribute('rel', 'noopener')
      } else {
        el.removeAttribute('target')
      }
    } else if (MEDIA_TAGS.has(node.nodeName)) {
      // Media only from the soma; nothing that loads from elsewhere or plays by itself.
      const el = node as Element
      if (!isFileURL(el.getAttribute('src') ?? '')) {
        el.parentNode?.removeChild(el)
        return
      }
      for (const { name } of [...el.attributes]) if (!MEDIA_ATTRS.has(name)) el.removeAttribute(name)
      for (const dim of ['width', 'height']) {
        const v = el.getAttribute(dim)
        if (v !== null && !/^\d{1,5}$/.test(v)) el.removeAttribute(dim)
      }
    } else if (node.nodeName === 'INPUT') {
      // Only task checkboxes: any other input becomes an inert checkbox.
      const el = node as Element
      for (const { name } of [...el.attributes]) if (!INPUT_ATTRS.has(name)) el.removeAttribute(name)
      el.setAttribute('type', 'checkbox')
    }
  })
  purifier = p
  return p
}

/** Renders note Markdown to sanitized HTML for the reading view. */
export function renderMarkdown(src: string, ctx: RenderContext): string {
  const p = purify()
  // Without a working sanitizer, never emit HTML: show the text instead.
  if (!p.isSupported) return `<pre>${escapeHtml(src)}</pre>`
  const html = renderer().render(src, { ctx })
  return p.sanitize(html, {
    FORBID_TAGS: ['style', 'form', 'button', 'textarea', 'select', 'option', 'iframe', 'frame', 'object', 'embed', 'svg', 'math', 'source', 'track', 'picture', 'link', 'meta', 'base'],
    FORBID_ATTR: ['style', 'srcset', 'formaction', 'xlink:href'],
    ADD_ATTR: ['target', 'controls', 'preload', 'loading'],
    ALLOW_DATA_ATTR: true,
  })
}
