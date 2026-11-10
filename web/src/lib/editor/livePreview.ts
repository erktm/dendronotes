// Live preview: Markdown is rendered in place while the line(s) with the
// cursor keep showing their syntax. Implemented as decorations over the
// Lezer syntax tree, so the document itself is never changed.

import { syntaxTree } from '@codemirror/language'
import { Facet, StateEffect, type Range } from '@codemirror/state'
import {
  Decoration,
  type DecorationSet,
  EditorView,
  ViewPlugin,
  type ViewUpdate,
  WidgetType,
} from '@codemirror/view'
import { isNoteTarget, splitWikiTarget } from '../synapses'
import { fileKind, fileURL, versionedFileURL } from '../files'
import { isNote } from '../paths'
import { strings } from '../strings'
import { isDrawingPath } from '../drawing/paths'

/** How the editor resolves and opens synapses (provided by the app). */
export interface SynapseHost {
  resolve(target: string, markdown: boolean): string | null
  open(target: string, markdown: boolean): void
  /** Resolves an attachment (image, PDF …) to a soma path. */
  resolveFile(target: string, markdown: boolean): string | null
  /** Opens a file of the soma (a drawing in its editor). */
  openFile?(path: string): void
  /** The version of a file (its mtime), so embeds load a changed file again. */
  fileVersion?(path: string): number | undefined
}

export const synapseHost = Facet.define<SynapseHost, SynapseHost | null>({
  combine: (values) => values[0] ?? null,
})

/** Dispatch to re-resolve synapses after the soma changed. */
export const refreshDecorations = StateEffect.define<null>()

const EXTERNAL = /^(https?:|mailto:)/i

class CheckboxWidget extends WidgetType {
  constructor(
    readonly checked: boolean,
    readonly pos: number,
  ) {
    super()
  }
  override eq(other: CheckboxWidget): boolean {
    return other.checked === this.checked && other.pos === this.pos
  }
  toDOM(view: EditorView): HTMLElement {
    const box = document.createElement('input')
    box.type = 'checkbox'
    box.className = 'cm-task-box'
    box.checked = this.checked
    box.setAttribute('aria-label', strings.editor.toggleTask)
    box.addEventListener('mousedown', (e) => {
      e.preventDefault()
      if (view.state.readOnly) return
      const at = this.pos + 1
      view.dispatch({ changes: { from: at, to: at + 1, insert: this.checked ? ' ' : 'x' } })
    })
    return box
  }
  override ignoreEvent(): boolean {
    return true
  }
}

class BulletWidget extends WidgetType {
  override eq(): boolean {
    return true
  }
  toDOM(): HTMLElement {
    const el = document.createElement('span')
    el.className = 'cm-bullet'
    el.textContent = '•'
    return el
  }
}

class RuleWidget extends WidgetType {
  override eq(): boolean {
    return true
  }
  toDOM(): HTMLElement {
    const el = document.createElement('span')
    el.className = 'cm-rule'
    return el
  }
}

class FenceWidget extends WidgetType {
  constructor(readonly lang: string) {
    super()
  }
  override eq(other: FenceWidget): boolean {
    return other.lang === this.lang
  }
  toDOM(): HTMLElement {
    const el = document.createElement('span')
    el.className = 'cm-fence-label'
    el.textContent = this.lang
    return el
  }
}

class ChipWidget extends WidgetType {
  constructor(
    readonly label: string,
    readonly kind: 'note' | 'attachment',
  ) {
    super()
  }
  override eq(other: ChipWidget): boolean {
    return other.label === this.label && other.kind === this.kind
  }
  toDOM(): HTMLElement {
    const el = document.createElement('span')
    el.className = `cm-chip cm-chip-${this.kind}`
    el.textContent = this.kind === 'note' ? `${strings.reading.embeddedNote} ${this.label}` : strings.reading.attachment(this.label)
    return el
  }
}

/** An image from the soma, shown in place of its Markdown. */
class ImageWidget extends WidgetType {
  constructor(
    readonly src: string,
    readonly alt: string,
    readonly width: number | undefined,
  ) {
    super()
  }
  override eq(other: ImageWidget): boolean {
    return other.src === this.src && other.alt === this.alt && other.width === this.width
  }
  toDOM(): HTMLElement {
    const img = document.createElement('img')
    img.className = 'cm-image'
    img.src = this.src
    img.alt = this.alt
    img.loading = 'lazy'
    img.draggable = false
    if (this.width) img.width = this.width
    return img
  }
  // Clicking the image puts the cursor there, which reveals the Markdown.
  override ignoreEvent(): boolean {
    return false
  }
}

/**
 * An embedded drawing, drawn from its scene. A click opens the editor; the
 * handle at the corner sets the width written after the pipe.
 */
class DrawingWidget extends WidgetType {
  constructor(
    readonly path: string,
    readonly label: string,
    readonly width: number | undefined,
    readonly version: number | undefined,
    readonly resizable: boolean,
  ) {
    super()
  }
  override eq(o: DrawingWidget): boolean {
    return o.path === this.path && o.label === this.label && o.width === this.width && o.version === this.version && o.resizable === this.resizable
  }
  toDOM(view: EditorView): HTMLElement {
    const host = document.createElement('span')
    host.className = 'cm-drawing drawing-embed'
    host.title = strings.drawing.openDrawing(this.label)
    const img = document.createElement('img')
    img.src = versionedFileURL(this.path, this.version)
    img.alt = this.label
    img.draggable = false
    if (this.width) img.width = this.width
    host.append(img)
    // The drawing code loads with the first drawing shown.
    void import('../drawing/embed').then((m) => m.fillDrawing(host, this.path, this.version, this.label))
    let resized = false
    host.addEventListener('click', (e) => {
      e.preventDefault()
      if (resized) {
        resized = false
        return
      }
      view.state.facet(synapseHost)?.openFile?.(this.path)
    })
    if (this.resizable) host.append(this.handle(view, host, () => (resized = true)))
    return host
  }
  /** The resize handle: dragging changes the shown width, letting go writes it. */
  private handle(view: EditorView, host: HTMLElement, done: () => void): HTMLElement {
    const grip = document.createElement('span')
    grip.className = 'cm-drawing-resize'
    grip.title = strings.drawing.resize
    grip.addEventListener('pointerdown', (e) => {
      e.preventDefault()
      e.stopPropagation()
      grip.setPointerCapture(e.pointerId)
      const startX = e.clientX
      const startW = host.getBoundingClientRect().width
      const max = view.contentDOM.getBoundingClientRect().width
      let width = Math.round(startW)
      host.classList.add('resizing')
      const move = (ev: PointerEvent) => {
        width = Math.round(Math.min(max, Math.max(48, startW + ev.clientX - startX)))
        for (const el of host.querySelectorAll('svg.dn-drawing, img')) el.setAttribute('width', String(width))
      }
      const up = () => {
        grip.removeEventListener('pointermove', move)
        grip.removeEventListener('pointerup', up)
        grip.removeEventListener('pointercancel', up)
        host.classList.remove('resizing')
        done()
        setWidth(view, host, width)
      }
      grip.addEventListener('pointermove', move)
      grip.addEventListener('pointerup', up)
      grip.addEventListener('pointercancel', up)
    })
    return grip
  }
  // The widget handles its own clicks; the Markdown shows when the cursor moves there.
  override ignoreEvent(): boolean {
    return true
  }
}

/** Writes a width into the embed a widget stands for: ![[Plan.drawing.svg|600]]. */
function setWidth(view: EditorView, dom: HTMLElement, width: number): void {
  let pos: number
  try {
    pos = view.posAtDOM(dom)
  } catch {
    return
  }
  const text = view.state.sliceDoc(pos, Math.min(view.state.doc.length, pos + 1024))
  const m = /^!\[\[([^\]|\n]+)(\|[^\]\n]*)?\]\]/.exec(text)
  if (!m || view.state.readOnly) return
  const insert = `![[${m[1]}|${width}]]`
  if (insert === m[0]) return
  view.dispatch({ changes: { from: pos, to: pos + m[0].length, insert }, userEvent: 'input' })
}

/** The widget for an embedded attachment: the image itself, or a chip. */
function attachmentWidget(
  host: SynapseHost | null,
  target: string,
  markdown: boolean,
  label: string,
  width?: number,
  resizable = false,
): WidgetType {
  const path = host?.resolveFile(target, markdown) ?? null
  if (path && isDrawingPath(path)) return new DrawingWidget(path, label, width, host?.fileVersion?.(path), resizable)
  if (path && fileKind(path) === 'image') return new ImageWidget(fileURL(path), label, width)
  return new ChipWidget(label || target, 'attachment')
}

const hidden = Decoration.replace({})
const bullet = Decoration.replace({ widget: new BulletWidget() })
const rule = Decoration.replace({ widget: new RuleWidget() })
const mark = (cls: string) => Decoration.mark({ class: cls })

/** Lines that show raw Markdown: those touched by a selection while focused. */
export function activeLines(view: EditorView): Set<number> {
  const lines = new Set<number>()
  if (!view.hasFocus) return lines
  for (const r of view.state.selection.ranges) {
    const first = view.state.doc.lineAt(r.from).number
    const last = view.state.doc.lineAt(r.to).number
    for (let n = first; n <= last; n++) lines.add(n)
  }
  return lines
}

function build(view: EditorView): DecorationSet {
  const { state } = view
  const host = state.facet(synapseHost)
  const active = activeLines(view)
  const out: Range<Decoration>[] = []
  const hide = (from: number, to: number) => {
    if (to > from) out.push(hidden.range(from, to))
  }
  const add = (deco: Decoration, from: number, to: number) => {
    if (to > from) out.push(deco.range(from, to))
  }
  const spaceAfter = (pos: number) => (state.sliceDoc(pos, pos + 1) === ' ' ? pos + 1 : pos)

  for (const { from, to } of view.visibleRanges) {
    syntaxTree(state).iterate({
      from,
      to,
      enter: (node) => {
        const line = state.doc.lineAt(node.from)
        const on = active.has(line.number)
        const name = node.name

        if (/^ATXHeading[1-6]$/.test(name)) {
          out.push(Decoration.line({ class: `cm-h cm-h${name.slice(-1)}` }).range(line.from))
          return
        }

        switch (name) {
          case 'HeaderMark':
            if (!on && node.node.parent?.name.startsWith('ATX')) hide(node.from, spaceAfter(node.to))
            return
          case 'Emphasis':
            add(mark('cm-em'), node.from, node.to)
            return
          case 'StrongEmphasis':
            add(mark('cm-strong'), node.from, node.to)
            return
          case 'Strikethrough':
            add(mark('cm-strike'), node.from, node.to)
            return
          case 'InlineCode':
            add(mark('cm-inline-code'), node.from, node.to)
            return
          case 'EmphasisMark':
          case 'StrikethroughMark':
            if (!on) hide(node.from, node.to)
            return
          case 'CodeMark':
            if (!on && node.node.parent?.name === 'InlineCode') hide(node.from, node.to)
            return
          case 'QuoteMark':
            if (!on) hide(node.from, spaceAfter(node.to))
            return
          case 'Blockquote': {
            const last = state.doc.lineAt(node.to).number
            for (let n = line.number; n <= last; n++) out.push(Decoration.line({ class: 'cm-quote' }).range(state.doc.line(n).from))
            return
          }
          case 'HorizontalRule':
            if (!on) out.push(rule.range(node.from, node.to))
            return
          case 'FencedCode': {
            const first = line.number
            const last = state.doc.lineAt(node.to).number
            for (let n = first; n <= last; n++) {
              const cls = n === first ? 'cm-codeblock cm-codeblock-start' : n === last ? 'cm-codeblock cm-codeblock-end' : 'cm-codeblock'
              out.push(Decoration.line({ class: cls }).range(state.doc.line(n).from))
            }
            if (!active.has(first)) {
              const info = node.node.getChild('CodeInfo')
              const l = state.doc.line(first)
              out.push(Decoration.replace({ widget: new FenceWidget(info ? state.sliceDoc(info.from, info.to) : '') }).range(l.from, l.to))
            }
            const closing = state.doc.line(last)
            if (last !== first && !active.has(last) && /^\s*(`{3,}|~{3,})\s*$/.test(closing.text)) hide(closing.from, closing.to)
            return false
          }
          case 'ListMark': {
            const item = node.node.parent
            if (on || !item) return
            if (item.getChild('Task')) hide(node.from, spaceAfter(node.to))
            else if (item.parent?.name === 'BulletList') out.push(bullet.range(node.from, node.to))
            return
          }
          case 'TaskMarker': {
            const checked = /x/i.test(state.sliceDoc(node.from, node.to))
            const task = node.node.parent
            if (checked && task) add(mark('cm-task-done'), node.to, task.to)
            if (!on) out.push(Decoration.replace({ widget: new CheckboxWidget(checked, node.from) }).range(node.from, node.to))
            return
          }
          case 'Image': {
            if (!on) {
              const text = state.sliceDoc(node.from, node.to)
              const m = /^!\[([^\]]*)\]\(\s*<?([^)\s>]*)/.exec(text)
              const alt = m?.[1] ?? ''
              let src = (m?.[2] ?? '').split('#')[0] ?? ''
              try {
                src = decodeURIComponent(src)
              } catch {
                // keep as written
              }
              const widget = EXTERNAL.test(src) || !src ? new ChipWidget(alt || text, 'attachment') : attachmentWidget(host, src, true, alt)
              out.push(Decoration.replace({ widget }).range(node.from, node.to))
            }
            return false
          }
          case 'Link': {
            const marks = node.node.getChildren('LinkMark')
            const urlNode = node.node.getChild('URL')
            if (marks.length < 2 || !urlNode) return false
            const labelFrom = marks[0]!.to
            const labelTo = marks[1]!.from
            const url = state.sliceDoc(urlNode.from, urlNode.to).replace(/^<|>$/g, '')
            if (EXTERNAL.test(url)) {
              add(Decoration.mark({ class: 'cm-link', attributes: { 'data-href': url } }), labelFrom, labelTo)
            } else {
              let path = url.split('#')[0] ?? ''
              try {
                path = decodeURIComponent(path)
              } catch {
                // keep as written
              }
              if (isNote(path)) {
                const resolved = host?.resolve(path, true) ?? null
                add(
                  Decoration.mark({
                    class: resolved ? 'cm-synapse' : 'cm-synapse cm-unresolved',
                    attributes: { 'data-target': path, 'data-markdown': 'true' },
                  }),
                  labelFrom,
                  labelTo,
                )
              } else {
                add(mark('cm-link'), labelFrom, labelTo)
              }
            }
            if (!on) {
              hide(node.from, labelFrom)
              hide(labelTo, node.to)
            }
            return false
          }
          case 'Synapse': {
            const embed = state.sliceDoc(node.from, node.from + 1) === '!'
            const innerFrom = node.from + (embed ? 3 : 2)
            const innerTo = node.to - 2
            const inner = state.sliceDoc(innerFrom, innerTo)
            const { target, heading, alias } = splitWikiTarget(inner)
            const isNoteLink = target !== '' && isNoteTarget(target)
            if (embed && !on) {
              const size = alias && /^(\d{1,5})(?:x\d{1,5})?$/.exec(alias.trim())
              const widget = isNoteLink
                ? new ChipWidget(alias ?? inner, 'note')
                : attachmentWidget(host, target, false, size ? target : (alias ?? target), size ? Number(size[1]) : undefined, !state.readOnly)
              out.push(Decoration.replace({ widget }).range(node.from, node.to))
              return false
            }
            const pipe = inner.indexOf('|')
            const labelFrom = pipe >= 0 ? innerFrom + pipe + 1 : innerFrom
            if (isNoteLink) {
              const resolved = host?.resolve(target, false) ?? null
              add(
                Decoration.mark({
                  class: resolved ? 'cm-synapse' : 'cm-synapse cm-unresolved',
                  attributes: { 'data-target': target, 'data-markdown': 'false', ...(heading ? { 'data-heading': heading } : {}) },
                }),
                labelFrom,
                innerTo,
              )
            } else {
              add(mark(target ? 'cm-attachment' : 'cm-link'), labelFrom, innerTo)
            }
            if (!on) {
              hide(node.from, labelFrom)
              hide(innerTo, node.to)
            }
            return false
          }
        }
      },
    })
  }

  for (const n of active) out.push(Decoration.line({ class: 'cm-md-active' }).range(state.doc.line(n).from))
  return Decoration.set(out, true)
}

const plugin = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet
    constructor(view: EditorView) {
      this.decorations = build(view)
    }
    update(u: ViewUpdate): void {
      if (
        u.docChanged ||
        u.viewportChanged ||
        u.selectionSet ||
        u.focusChanged ||
        syntaxTree(u.startState) !== syntaxTree(u.state) ||
        u.transactions.some((tr) => tr.effects.some((e) => e.is(refreshDecorations)))
      ) {
        this.decorations = build(u.view)
      }
    }
  },
  { decorations: (v) => v.decorations },
)

/** Opens synapses and external links on click (with Cmd/Ctrl on raw lines). */
const clicks = EditorView.domEventHandlers({
  mousedown(event, view) {
    const el = (event.target as HTMLElement | null)?.closest<HTMLElement>('.cm-synapse, .cm-link[data-href]')
    if (!el || event.button !== 0) return false
    const line = view.state.doc.lineAt(view.posAtDOM(el)).number
    if (activeLines(view).has(line) && !(event.metaKey || event.ctrlKey)) return false
    event.preventDefault()
    const href = el.dataset.href
    if (href) {
      if (EXTERNAL.test(href)) window.open(href, '_blank', 'noopener,noreferrer')
    } else {
      view.state.facet(synapseHost)?.open(el.dataset.target ?? '', el.dataset.markdown === 'true')
    }
    return true
  },
})

export const livePreview = [plugin, clicks]
