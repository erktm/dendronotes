<script lang="ts" module>
  // The look of the next element, kept across drawings in this tab.
  import { DEFAULT_STYLE, type Style } from '../lib/drawing/style'
  let lastStyle: Style = { ...DEFAULT_STYLE }
  // Copied elements, also from one drawing into another.
  import type { Element as DrawingElement, FileData } from '../lib/drawing/scene'
  let clipboard: { elements: DrawingElement[]; files: Record<string, FileData> } | null = null
</script>

<script lang="ts">
  // The drawing editor: a canvas of SVG built from the scene (lib/drawing),
  // tools on the left, properties on the right. Pointer events work for
  // mouse, pen and touch; two fingers move and zoom the canvas.
  import { onDestroy, onMount, tick } from 'svelte'
  import Icon, { type IconName } from './Icon.svelte'
  import EmptyState from './EmptyState.svelte'
  import AttachmentView from './AttachmentView.svelte'
  import DrawingProps from './DrawingProps.svelte'
  import IconLibrary from './IconLibrary.svelte'
  import { DrawingDoc } from '../lib/drawing/doc.svelte'
  import { SceneLayer, measureText } from '../lib/drawing/dom'
  import { newIcon, newImage, newLine, newPen, newShape, newText } from '../lib/drawing/create'
  import { type Box, type Pt, boxFrom, center, dist, grow, simplify, snap } from '../lib/drawing/geometry'
  import { ALL_HANDLES, CORNERS, type Handle, bindTargetAt, elementsIn, handlePoint, hitTest, resizeBox, shapeAt } from '../lib/drawing/hit'
  import { LINE_HEIGHT, labelArea, midpoint, textBox } from '../lib/drawing/render'
  import {
    type LinearEl,
    type Order,
    type Scene,
    type ShapeType,
    add,
    bounds,
    copies,
    find,
    fit,
    hasText,
    isBox,
    isLinear,
    isShape,
    isShapeType,
    move,
    newId,
    remove,
    reorder,
    replace,
    route,
    sceneBounds,
  } from '../lib/drawing/scene'
  import { keysOf, restyle, styleOf, type StyleKey } from '../lib/drawing/style'
  import { drawingTitle } from '../lib/drawing/paths'
  import { app } from '../lib/state/app.svelte'
  import { cssVars, focusOnMount } from '../lib/actions'
  import { errorMessage } from '../lib/errors'
  import { ApiError } from '../lib/api/client'
  import { strings } from '../lib/strings'
  import { basename } from '../lib/paths'
  import ContextMenu, { type MenuItem } from './ContextMenu.svelte'
  import { pngBlob, saveFile, svgBlob } from '../lib/drawing/export'

  let {
    path,
    readonly,
    ondone,
    doneLabel = strings.drawing.done,
    onedit,
  }: {
    path: string
    readonly: boolean
    ondone: () => void
    doneLabel?: string
    /** Offered while only viewing: switch to editing. */
    onedit?: () => void
  } = $props()

  type Tool = 'select' | 'hand' | ShapeType | 'line' | 'arrow' | 'pen' | 'text'

  const GRID = 20
  /** Handle size on screen. */
  const HANDLE = 10
  const s = strings.drawing
  const measure = measureText()
  // svelte-ignore state_referenced_locally
  const doc = new DrawingDoc(path)

  let tool = $state<Tool>('select')
  let selected = $state.raw<ReadonlySet<string>>(new Set())
  let style = $state<Style>({ ...lastStyle })
  let grid = $state(readGrid())
  let iconsOpen = $state(false)
  // The shapes beyond the first three sit in a group that opens below its button.
  const MORE_SHAPES = ['triangle', 'hexagon', 'parallelogram', 'cylinder', 'star', 'cloud'] as const satisfies readonly ShapeType[]
  type MoreShape = (typeof MORE_SHAPES)[number]
  const isMoreShape = (t: Tool): t is MoreShape => (MORE_SHAPES as readonly string[]).includes(t)
  let shapesOpen = $state(false)
  let moreShape = $state<MoreShape>('triangle')
  let propsOpen = $state(typeof window === 'undefined' || window.innerWidth > 760)
  let pan = $state({ x: 40, y: 40 })
  let zoom = $state(1)
  let marquee = $state.raw<Box | null>(null)
  let bindHint = $state<string | null>(null)
  let spaceDown = $state(false)
  let panning = $state(false)
  let editing = $state.raw<{ id: string; before: Scene; created: boolean } | null>(null)
  let editText = $state('')

  let stage = $state<HTMLDivElement>()
  let svg = $state<SVGSVGElement>()
  let layerEl = $state<SVGGElement>()
  let layer: SceneLayer | null = null

  const mayEdit = $derived(!readonly && app.can.write)
  const name = $derived(drawingTitle(path))

  // --- How to show the drawing in a note ------------------------------------

  /** Embed syntax inside the help text, shown as code. */
  const EMBED_SAMPLE = /(!\[\[[^\]\s]*(?:\]\])?)/
  let embedOpen = $state(false)
  let embedWrap = $state<HTMLElement>()
  /** The embed to write: the file name, or the path when the name is not unique. */
  const embedCode = $derived.by(() => {
    const file = basename(path)
    const same = app.filePaths.filter((p) => basename(p).toLowerCase() === file.toLowerCase()).length
    return `![[${same <= 1 ? file : path}]]`
  })

  // --- Download -------------------------------------------------------------

  let downloadMenu = $state<{ x: number; y: number; items: MenuItem[] } | null>(null)

  function openDownload(e: MouseEvent) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    downloadMenu = {
      x: r.left,
      y: r.bottom + 4,
      items: [
        { label: s.downloadSVG, icon: 'drawing', action: () => saveFile(svgBlob(doc.scene), basename(path)) },
        { label: s.downloadPNG, icon: 'image', action: () => void downloadPNG() },
      ],
    }
  }

  /** A picture as the app shows the drawing now: light or dark. */
  async function downloadPNG() {
    const theme = document.documentElement.dataset.theme
    const dark = theme === 'dark' || (theme !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)
    try {
      saveFile(await pngBlob(doc.scene, dark ? 'dark' : 'light'), `${name}.png`)
    } catch {
      app.toast(s.downloadFailed)
    }
  }

  async function copyEmbed() {
    try {
      await navigator.clipboard.writeText(embedCode)
      app.toast(s.embed.copied, 'info')
    } catch {
      app.toast(s.embed.copyFailed)
    }
  }

  // --- Saved settings of this browser -------------------------------------

  function readGrid(): boolean {
    try {
      return localStorage.getItem('dendronotes.drawing.grid') === '1'
    } catch {
      return false
    }
  }

  function setGrid(on: boolean) {
    grid = on
    try {
      localStorage.setItem('dendronotes.drawing.grid', on ? '1' : '0')
    } catch {
      // Only for this visit then.
    }
  }

  $effect(() => {
    lastStyle = { ...style }
  })

  // --- Loading, rendering, saving -------------------------------------------

  onMount(() => {
    void doc.load().then(async () => {
      await tick()
      // Only viewing: as large as fits; editing: never larger than it is.
      fitView(readonly ? 3 : 1)
    })
    // Text is measured again once the fonts are there.
    void document.fonts?.ready.then(() => layer?.refresh(shownScene))
  })

  onDestroy(() => doc.close())

  $effect(() => {
    if (layerEl && !layer) layer = new SceneLayer(layerEl, measure)
    layer?.render(shownScene)
  })

  // Changes saved elsewhere arrive through the tree.
  let lastMtime: number | undefined
  $effect(() => {
    const mtime = app.entries.find((e) => e.path === path)?.mtime
    if (lastMtime !== undefined && mtime !== lastMtime) void doc.changedElsewhere()
    lastMtime = mtime
  })

  const scene = $derived(doc.scene)
  /** What is drawn: while text is edited, the text box shows it instead. */
  const shownScene = $derived.by(() => {
    if (!editing) return doc.scene
    const el = find(doc.scene, editing.id)
    return el && hasText(el) ? replace(doc.scene, { ...el, text: '' }) : doc.scene
  })

  function done() {
    void doc.flush()
    ondone()
  }

  // --- Selection ----------------------------------------------------------

  const selection = $derived(scene.elements.filter((e) => selected.has(e.id)))
  const single = $derived(selection.length === 1 ? selection[0]! : null)
  /** The box handles resize: a single shape, text or picture by itself, else the selection's bounds. */
  const frame = $derived<Box | null>(
    single && isBox(single) ? { x: single.x, y: single.y, w: single.w, h: single.h } : selection.length ? sceneBounds(scene, selected) : null,
  )
  const handles = $derived<readonly Handle[]>(
    !mayEdit || !frame || (single && isLinear(single)) ? [] : single?.type === 'text' || selection.length > 1 ? CORNERS : ALL_HANDLES,
  )

  // Drop ids that are gone (undo, someone else's version).
  $effect(() => {
    if ([...selected].some((id) => !find(scene, id))) selected = new Set([...selected].filter((id) => find(scene, id)))
  })

  const panelKeys = $derived.by((): ReadonlySet<StyleKey> => {
    if (selection.length) return new Set(selection.flatMap(keysOf))
    if (isShapeType(tool)) return new Set(keysOf(newShape(tool, { x: 0, y: 0, w: 0, h: 0 }, style)))
    const probe: Record<string, StyleKey[]> = {
      line: ['stroke', 'width', 'dash', 'elbow', 'font', 'size'],
      arrow: ['stroke', 'width', 'dash', 'startHead', 'endHead', 'elbow', 'font', 'size'],
      pen: ['stroke', 'width'],
      text: ['stroke', 'font', 'size', 'align'],
    }
    return new Set(probe[tool] ?? [])
  })
  const panelStyle = $derived<Style>(single || selection[0] ? { ...style, ...styleOf(selection[0]!) } : style)

  function setStyle(patch: Partial<Style>) {
    style = { ...style, ...patch }
    if (!mayEdit || selection.length === 0) return
    doc.commit(replace(scene, ...selection.map((el) => restyle(el, patch, measure))))
  }

  function setOrder(order: Order) {
    if (mayEdit && selected.size) doc.commit(reorder(scene, selected, order))
  }

  function duplicate() {
    if (!mayEdit || !selection.length) return
    const made = copies(selection, 20)
    doc.commit(add(scene, ...made))
    selected = new Set(made.map((e) => e.id))
  }

  function deleteSelection() {
    if (!mayEdit || !selected.size) return
    doc.commit(remove(scene, selected))
    selected = new Set()
  }

  function setSketchy(on: boolean) {
    if (mayEdit) doc.commit({ ...scene, sketchy: on })
  }

  // --- View: pan and zoom -------------------------------------------------

  function stageRect(): DOMRect {
    return stage!.getBoundingClientRect()
  }

  function toScene(clientX: number, clientY: number): Pt {
    const r = stageRect()
    return [(clientX - r.left - pan.x) / zoom, (clientY - r.top - pan.y) / zoom]
  }

  function zoomAt(sx: number, sy: number, z: number) {
    const next = Math.min(8, Math.max(0.1, z))
    pan = { x: sx - ((sx - pan.x) * next) / zoom, y: sy - ((sy - pan.y) * next) / zoom }
    zoom = next
  }

  function zoomBy(factor: number) {
    const r = stageRect()
    zoomAt(r.width / 2, r.height / 2, zoom * factor)
  }

  /** Shows the whole drawing (not larger than maxZoom). */
  function fitView(maxZoom = 2) {
    if (!stage) return
    const r = stageRect()
    const b = sceneBounds(scene)
    if (!b) {
      zoom = 1
      pan = { x: 40, y: 40 }
      return
    }
    const z = Math.min(maxZoom, Math.max(0.1, Math.min((r.width - 80) / Math.max(b.w, 1), (r.height - 80) / Math.max(b.h, 1))))
    zoom = z
    pan = { x: r.width / 2 - (b.x + b.w / 2) * z, y: r.height / 2 - (b.y + b.h / 2) * z }
  }

  onMount(() => {
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      const r = stageRect()
      if (e.ctrlKey || e.metaKey) zoomAt(e.clientX - r.left, e.clientY - r.top, zoom * Math.exp(-e.deltaY * 0.01))
      else pan = { x: pan.x - e.deltaX, y: pan.y - e.deltaY }
    }
    stage?.addEventListener('wheel', onWheel, { passive: false })
    return () => stage?.removeEventListener('wheel', onWheel)
  })

  // --- Gestures -----------------------------------------------------------

  type Gesture =
    | { kind: 'pan'; sx: number; sy: number; x: number; y: number }
    | { kind: 'move'; start: Pt; before: Scene; base: Scene; ids: ReadonlySet<string>; box: Box; moved: boolean }
    | { kind: 'resize'; handle: Handle; frame: Box; before: Scene; ids: ReadonlySet<string> }
    | { kind: 'end'; which: 0 | 1; el: LinearEl; before: Scene }
    | { kind: 'shape'; type: ShapeType; id: string; start: Pt; before: Scene }
    | { kind: 'line'; el: LinearEl; before: Scene; exclude: ReadonlySet<string> }
    | { kind: 'pen'; points: Pt[]; id: string; before: Scene }
    | { kind: 'marquee'; start: Pt; base: ReadonlySet<string> }
    | { kind: 'pinch'; d: number; mid: Pt; zoom: number; pan: { x: number; y: number } }

  let gesture: Gesture | null = null
  const pointers = new Map<number, Pt>()

  const snapPt = (p: Pt): Pt => (grid ? [snap(p[0], GRID), snap(p[1], GRID)] : p)

  function cancelGesture() {
    if (gesture && 'before' in gesture) doc.preview(gesture.before)
    gesture = null
    marquee = null
    bindHint = null
    panning = false
  }

  function startPinch() {
    const [a, b] = [...pointers.values()]
    const r = stageRect()
    gesture = {
      kind: 'pinch',
      d: Math.max(dist(a!, b!), 1),
      mid: [(a![0] + b![0]) / 2 - r.left, (a![1] + b![1]) / 2 - r.top],
      zoom,
      pan: { ...pan },
    }
  }

  function down(e: PointerEvent) {
    if (editing) finishText()
    pointers.set(e.pointerId, [e.clientX, e.clientY])
    svg?.setPointerCapture(e.pointerId)
    if (pointers.size === 2) {
      cancelGesture()
      startPinch()
      return
    }
    if (pointers.size > 2) return
    e.preventDefault()
    stage?.focus({ preventScroll: true })
    const p = toScene(e.clientX, e.clientY)
    if (e.button === 1 || tool === 'hand' || spaceDown || !mayEdit) {
      gesture = { kind: 'pan', sx: e.clientX, sy: e.clientY, x: pan.x, y: pan.y }
      panning = true
      return
    }
    if (e.button !== 0) return
    const before = scene
    switch (tool) {
      case 'select':
        return selectDown(e, p)
      case 'rect':
      case 'ellipse':
      case 'diamond':
      case 'triangle':
      case 'hexagon':
      case 'parallelogram':
      case 'cylinder':
      case 'star':
      case 'cloud': {
        const start = snapPt(p)
        const el = newShape(tool, { x: start[0], y: start[1], w: 0, h: 0 }, style)
        doc.preview(add(before, el))
        gesture = { kind: 'shape', type: tool, id: el.id, start, before }
        return
      }
      case 'line':
      case 'arrow': {
        const target = bindTargetAt(before, p, 10 / zoom, new Set())
        const start = snapPt(p)
        const el = newLine(tool, start, start, style, target ? { id: target.id } : null, null)
        doc.preview(add(before, el))
        gesture = { kind: 'line', el, before, exclude: new Set(target ? [target.id] : []) }
        return
      }
      case 'pen': {
        const el = newPen([p], style)
        doc.preview(add(before, el))
        gesture = { kind: 'pen', points: [p], id: el.id, before }
        return
      }
      case 'text': {
        const hit = hitTest(before, p, 6 / zoom, measure) ?? shapeAt(before, p)
        if (hit && hasText(hit)) startEditing(hit.id)
        else createText(p)
        return
      }
    }
  }

  function selectDown(e: PointerEvent, p: Pt) {
    const before = scene
    const reach = HANDLE / zoom
    if (frame) {
      for (const h of handles) {
        const hp = handlePoint(frame, h)
        if (Math.abs(p[0] - hp[0]) <= reach && Math.abs(p[1] - hp[1]) <= reach) {
          gesture = { kind: 'resize', handle: h, frame, before, ids: selected }
          return
        }
      }
    }
    if (mayEdit && single && isLinear(single)) {
      const pts = route(single, before)
      for (const which of [0, 1] as const) {
        const end = which === 0 ? pts[0]! : pts[pts.length - 1]!
        if (dist(p, end) <= reach) {
          gesture = { kind: 'end', which, el: single, before }
          return
        }
      }
    }
    const hit = hitTest(before, p, 6 / zoom, measure)
    if (!hit) {
      if (!e.shiftKey) selected = new Set()
      gesture = { kind: 'marquee', start: p, base: selected }
      return
    }
    if (e.shiftKey) {
      const next = new Set(selected)
      if (next.has(hit.id)) {
        next.delete(hit.id)
        selected = next
        return
      }
      next.add(hit.id)
      selected = next
    } else if (!selected.has(hit.id)) {
      selected = new Set([hit.id])
    }
    let ids = selected
    let base = before
    // Alt-drag moves a copy.
    if (e.altKey && mayEdit) {
      const made = copies(before.elements.filter((x) => ids.has(x.id)), 0)
      base = add(before, ...made)
      ids = new Set(made.map((x) => x.id))
    }
    const box = sceneBounds(base, ids) ?? { x: p[0], y: p[1], w: 0, h: 0 }
    gesture = { kind: 'move', start: p, before, base, ids, box, moved: false }
  }

  function moveEvent(e: PointerEvent) {
    if (pointers.has(e.pointerId)) pointers.set(e.pointerId, [e.clientX, e.clientY])
    const g = gesture
    if (!g) return
    if (g.kind === 'pinch') {
      if (pointers.size < 2) return
      const [a, b] = [...pointers.values()]
      const r = stageRect()
      const mid: Pt = [(a![0] + b![0]) / 2 - r.left, (a![1] + b![1]) / 2 - r.top]
      const z = Math.min(8, Math.max(0.1, (g.zoom * dist(a!, b!)) / g.d))
      pan = { x: mid[0] - ((g.mid[0] - g.pan.x) * z) / g.zoom, y: mid[1] - ((g.mid[1] - g.pan.y) * z) / g.zoom }
      zoom = z
      return
    }
    if (g.kind === 'pan') {
      pan = { x: g.x + e.clientX - g.sx, y: g.y + e.clientY - g.sy }
      return
    }
    const p = toScene(e.clientX, e.clientY)
    switch (g.kind) {
      case 'move': {
        let dx = p[0] - g.start[0]
        let dy = p[1] - g.start[1]
        if (!g.moved && Math.hypot(dx, dy) * zoom < 3) return
        if (!mayEdit) return
        if (!g.moved && g.base !== g.before) selected = g.ids
        g.moved = true
        if (grid) {
          dx = snap(g.box.x + dx, GRID) - g.box.x
          dy = snap(g.box.y + dy, GRID) - g.box.y
        }
        doc.preview(move(g.base, g.ids, dx, dy))
        return
      }
      case 'resize':
        return resizeTo(g, snapPt(p), e.shiftKey)
      case 'end': {
        const target = bindTargetAt(g.before, p, 10 / zoom, new Set([g.el.id]))
        const pt = snapPt(p)
        const pts: [Pt, Pt] = g.which === 0 ? [pt, g.el.points[1]] : [g.el.points[0], pt]
        const bind = target ? { id: target.id } : null
        const next = g.which === 0 ? { ...g.el, points: pts, start: bind } : { ...g.el, points: pts, end: bind }
        doc.preview(replace(g.before, next))
        bindHint = target?.id ?? null
        return
      }
      case 'shape': {
        let b = boxFrom(g.start, snapPt(p))
        if (e.shiftKey) {
          const side = Math.max(b.w, b.h)
          b = { x: p[0] < g.start[0] ? g.start[0] - side : g.start[0], y: p[1] < g.start[1] ? g.start[1] - side : g.start[1], w: side, h: side }
        }
        const el = find(scene, g.id)
        if (el && isShape(el)) doc.preview(add(g.before, { ...el, ...b }))
        return
      }
      case 'line': {
        const target = bindTargetAt(g.before, p, 10 / zoom, g.exclude)
        let end = snapPt(p)
        if (e.shiftKey && !target) {
          // Steps of 15 degrees.
          const [a] = g.el.points
          const angle = Math.round(Math.atan2(end[1] - a[1], end[0] - a[0]) / (Math.PI / 12)) * (Math.PI / 12)
          const len = dist(a, end)
          end = [a[0] + Math.cos(angle) * len, a[1] + Math.sin(angle) * len]
        }
        g.el = { ...g.el, points: [g.el.points[0], end], end: target ? { id: target.id } : null }
        doc.preview(add(g.before, g.el))
        bindHint = target?.id ?? null
        return
      }
      case 'pen': {
        const events = e.getCoalescedEvents?.() ?? [e]
        for (const ev of events.length ? events : [e]) {
          const q = toScene(ev.clientX, ev.clientY)
          if (dist(q, g.points[g.points.length - 1]!) * zoom >= 1.5) g.points.push(q)
        }
        const el = find(scene, g.id)
        if (el?.type === 'pen') doc.preview(add(g.before, { ...el, points: g.points }))
        return
      }
      case 'marquee': {
        const box = boxFrom(g.start, p)
        marquee = box
        selected = new Set([...g.base, ...elementsIn(scene, box)])
        return
      }
    }
  }

  function resizeTo(g: Extract<Gesture, { kind: 'resize' }>, p: Pt, shift: boolean) {
    const els = g.before.elements.filter((x) => g.ids.has(x.id))
    const one = els.length === 1 ? els[0]! : null
    const keepRatio = shift || !one || one.type === 'image' || one.type === 'icon' || one.type === 'text'
    const nb = resizeBox(g.frame, g.handle, p, keepRatio, 8)
    if (one?.type === 'text') {
      const size = Math.min(200, Math.max(6, (one.size * nb.h) / Math.max(g.frame.h, 1)))
      const { w, h } = textBox({ ...one, size }, measure)
      const x = g.handle.includes('w') ? g.frame.x + g.frame.w - w : g.frame.x
      const y = g.handle.includes('n') ? g.frame.y + g.frame.h - h : g.frame.y
      doc.preview(replace(g.before, { ...one, size: Math.round(size * 10) / 10, x, y, w, h }))
      return
    }
    doc.preview(replace(g.before, ...els.map((x) => fit(x, g.frame, nb))))
  }

  function up(e: PointerEvent) {
    pointers.delete(e.pointerId)
    const g = gesture
    if (!g) return
    if (g.kind === 'pinch') {
      if (pointers.size < 2) gesture = null
      return
    }
    gesture = null
    marquee = null
    bindHint = null
    panning = false
    switch (g.kind) {
      case 'move':
        if (g.moved) doc.commit(scene, g.before)
        return
      case 'resize':
      case 'end':
        doc.commit(scene, g.before)
        return
      case 'shape': {
        let el = find(scene, g.id)
        if (!el || !isShape(el)) return
        if (el.w * zoom < 6 && el.h * zoom < 6) {
          // A click: a shape of the usual size there.
          el = { ...el, x: g.start[0] - 60, y: g.start[1] - 40, w: 120, h: 80 }
        }
        doc.commit(add(g.before, el), g.before)
        selected = new Set([el.id])
        tool = 'select'
        return
      }
      case 'line': {
        const [a, b] = g.el.points
        if (dist(a, b) * zoom < 6 && !g.el.end) {
          doc.preview(g.before)
          return
        }
        doc.commit(scene, g.before)
        selected = new Set([g.el.id])
        tool = 'select'
        return
      }
      case 'pen': {
        const el = find(scene, g.id)
        if (el?.type !== 'pen') return
        doc.commit(add(g.before, { ...el, points: simplify(g.points, 0.6 / zoom) }), g.before)
        return
      }
    }
  }

  function dblclick(e: MouseEvent) {
    if (!mayEdit || tool !== 'select') return
    const p = toScene(e.clientX, e.clientY)
    // Inside an empty shape too: its label.
    const hit = hitTest(scene, p, 6 / zoom, measure) ?? shapeAt(scene, p)
    if (hit && hasText(hit)) startEditing(hit.id)
    else if (!hit) createText(p)
  }

  // --- Text ---------------------------------------------------------------

  function createText(p: Pt) {
    const before = scene
    const el = newText(snapPt(p), style, measure)
    doc.preview(add(before, el))
    editing = { id: el.id, before, created: true }
    editText = ''
    selected = new Set([el.id])
  }

  function startEditing(id: string) {
    const el = find(scene, id)
    if (!el || !hasText(el) || !mayEdit) return
    editing = { id, before: scene, created: false }
    editText = el.text
    selected = new Set([id])
  }

  function typed(text: string) {
    editText = text
    const el = editing && find(scene, editing.id)
    if (!el || !hasText(el)) return
    const next = el.type === 'text' ? { ...el, text, ...textBox({ ...el, text }, measure) } : { ...el, text }
    doc.preview(replace(scene, next))
  }

  function finishText() {
    const ed = editing
    if (!ed) return
    editing = null
    const el = find(scene, ed.id)
    if (!el) return
    if (el.type === 'text' && el.text.trim() === '') {
      if (ed.created) doc.preview(ed.before)
      else doc.commit(remove(scene, new Set([el.id])), ed.before)
      selected = new Set()
    } else {
      doc.commit(scene, ed.before)
    }
    tool = 'select'
    stage?.focus({ preventScroll: true })
  }

  /** Where the text box sits on screen, in the element's font. */
  const editBox = $derived.by(() => {
    const el = editing && find(scene, editing.id)
    if (!el || !hasText(el)) return null
    const lh = el.size * LINE_HEIGHT
    const lines = Math.max(1, editText.split('\n').length)
    const widest = Math.max(...editText.split('\n').map((l) => measure(l, el.font, el.size)), el.size)
    let x: number
    let y: number
    let w: number
    let align = el.align
    if (el.type === 'text') {
      x = el.x
      y = el.y
      w = widest + el.size
    } else if (isShape(el)) {
      const area = labelArea(el)
      w = Math.max(area.w, el.size * 2)
      x = area.x + area.w / 2 - w / 2
      y = area.y + area.h / 2 - (lines * lh) / 2
    } else {
      const [mx, my] = midpoint(route(el, scene))
      w = Math.max(widest + el.size, 80)
      x = mx - w / 2
      y = my - (lines * lh) / 2
      align = 'center'
    }
    return {
      '--x': `${x * zoom + pan.x}px`,
      '--y': `${y * zoom + pan.y}px`,
      '--w': `${w * zoom + 4}px`,
      '--h': `${lines * lh * zoom + 4}px`,
      '--size': `${el.size * zoom}px`,
      '--align': align,
      '--family': el.font === 'serif' ? 'var(--font-serif)' : el.font === 'mono' ? 'var(--font-mono)' : 'var(--font-ui)',
      '--ink': el.stroke === 'ink' ? 'var(--text)' : `var(--draw-${el.stroke})`,
    }
  })

  function textKey(e: KeyboardEvent) {
    if (e.key === 'Escape' || (e.key === 'Enter' && (e.metaKey || e.ctrlKey))) {
      e.preventDefault()
      e.stopPropagation()
      finishText()
    }
  }

  // --- Icons --------------------------------------------------------------

  function insertPicture(file: FileData) {
    if (!mayEdit) return
    const r = stageRect()
    const c = toScene(r.left + r.width / 2, r.top + r.height / 2)
    const k = Math.min(1, 96 / Math.max(file.w, file.h))
    const w = Math.max(16, file.w * k)
    const h = Math.max(16, file.h * k)
    const key = newId()
    const el = newImage(key, { x: c[0] - w / 2, y: c[1] - h / 2, w, h })
    doc.commit(add({ ...scene, files: { ...scene.files, [key]: file } }, el))
    selected = new Set([el.id])
    tool = 'select'
  }

  /** Puts a built-in icon in the middle of the view, in the current color and width. */
  function insertIcon(name: string) {
    if (!mayEdit) return
    const r = stageRect()
    const c = toScene(r.left + r.width / 2, r.top + r.height / 2)
    const size = 64
    const el = newIcon(name, { x: c[0] - size / 2, y: c[1] - size / 2, w: size, h: size }, style)
    doc.commit(add(scene, el))
    selected = new Set([el.id])
    tool = 'select'
  }

  // --- Keyboard -----------------------------------------------------------

  const TOOL_KEYS: Record<string, Tool> = { v: 'select', '1': 'select', h: 'hand', r: 'rect', o: 'ellipse', d: 'diamond', l: 'line', a: 'arrow', p: 'pen', t: 'text' }

  function key(e: KeyboardEvent) {
    const target = e.target as HTMLElement | null
    if (target?.closest('input, textarea, select, [contenteditable]')) return
    // Buttons keep Enter and Space.
    if (target?.closest('button') && (e.key === 'Enter' || e.key === ' ')) return
    const mod = e.metaKey || e.ctrlKey
    const k = e.key.toLowerCase()
    if (e.key === ' ') {
      spaceDown = true
      e.preventDefault()
      return
    }
    if (mod && k === 'z') {
      e.preventDefault()
      if (!mayEdit) return
      if (e.shiftKey) doc.redo()
      else doc.undo()
      return
    }
    if (mod && k === 'y') {
      e.preventDefault()
      if (mayEdit) doc.redo()
      return
    }
    if (mod && k === 'a') {
      e.preventDefault()
      selected = new Set(scene.elements.map((x) => x.id))
      return
    }
    if (mod && k === 'c') {
      if (!selection.length) return
      e.preventDefault()
      const files = Object.fromEntries(selection.flatMap((x) => (x.type === 'image' && scene.files[x.file] ? [[x.file, scene.files[x.file]!]] : [])))
      clipboard = { elements: selection, files }
      return
    }
    if (mod && k === 'v') {
      if (!clipboard || !mayEdit) return
      e.preventDefault()
      const made = copies(clipboard.elements, 20)
      doc.commit(add({ ...scene, files: { ...scene.files, ...clipboard.files } }, ...made))
      selected = new Set(made.map((x) => x.id))
      clipboard = { ...clipboard, elements: made }
      return
    }
    if (mod && k === 'd') {
      e.preventDefault()
      duplicate()
      return
    }
    if (mod && (e.key === ']' || e.key === '[')) {
      e.preventDefault()
      setOrder(e.key === ']' ? (e.shiftKey ? 'front' : 'forward') : e.shiftKey ? 'back' : 'backward')
      return
    }
    if (mod) return
    if (e.key === 'Escape') {
      if (embedOpen) embedOpen = false
      else if (gesture) cancelGesture()
      else if (iconsOpen) iconsOpen = false
      else if (tool !== 'select') tool = 'select'
      else selected = new Set()
      return
    }
    if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault()
      deleteSelection()
      return
    }
    if (e.key.startsWith('Arrow') && selected.size && mayEdit) {
      e.preventDefault()
      const step = e.shiftKey ? 10 : 1
      const dx = e.key === 'ArrowLeft' ? -step : e.key === 'ArrowRight' ? step : 0
      const dy = e.key === 'ArrowUp' ? -step : e.key === 'ArrowDown' ? step : 0
      doc.commit(move(scene, selected, dx, dy))
      return
    }
    if (e.key === 'Enter' && single && hasText(single)) {
      e.preventDefault()
      startEditing(single.id)
      return
    }
    if (e.key === '+' || e.key === '=') return zoomBy(1.2)
    if (e.key === '-') return zoomBy(1 / 1.2)
    if (e.key === '!' || (e.shiftKey && e.code === 'Digit1')) return fitView()
    if (k === 'g') return setGrid(!grid)
    if (k === 'i' && mayEdit) {
      iconsOpen = !iconsOpen
      return
    }
    const t = TOOL_KEYS[k]
    if (t && (mayEdit || t === 'hand')) tool = t
  }

  function keyUp(e: KeyboardEvent) {
    if (e.key === ' ') spaceDown = false
  }

  // --- Overlay geometry ---------------------------------------------------

  const hs = $derived(HANDLE / zoom)
  const hintBox = $derived.by(() => {
    const el = bindHint ? find(scene, bindHint) : undefined
    return el && isBox(el) ? grow(bounds(el, scene), 4) : null
  })
  const outlines = $derived(selection.length > 1 ? selection.map((x) => grow(bounds(x, scene), 3)) : [])
  const frameBox = $derived(frame ? grow(frame, single && isLinear(single) ? 0 : 3 / zoom) : null)
  const lineEnds = $derived.by((): Pt[] => {
    if (!mayEdit || !single || !isLinear(single)) return []
    const pts = route(single, scene)
    return [pts[0]!, pts[pts.length - 1]!]
  })

  const tools: { id: Tool; icon: IconName; label: string }[] = [
    { id: 'select', icon: 'pointer', label: s.tool.select },
    { id: 'hand', icon: 'move', label: s.tool.hand },
    { id: 'rect', icon: 'square', label: s.tool.rect },
    { id: 'ellipse', icon: 'circle', label: s.tool.ellipse },
    { id: 'diamond', icon: 'diamond', label: s.tool.diamond },
    { id: 'line', icon: 'line', label: s.tool.line },
    { id: 'arrow', icon: 'arrow', label: s.tool.arrow },
    { id: 'pen', icon: 'pencil', label: s.tool.pen },
    { id: 'text', icon: 'text', label: s.tool.text },
  ]

  const saveText = $derived(
    !mayEdit
      ? s.readOnly
      : doc.save === 'saving'
        ? s.saving
        : doc.save === 'pending'
          ? s.pending
          : doc.save === 'failed'
            ? s.failed(errorMessage(new ApiError(0, doc.failure || 'network')))
            : doc.save === 'conflict'
              ? s.conflict
              : s.saved,
  )

  const stageVars = $derived({
    '--grid': `${GRID * zoom}px`,
    '--gx': `${pan.x}px`,
    '--gy': `${pan.y}px`,
  })
</script>

<svelte:window
  onkeyup={keyUp}
  onpointerdown={(e) => {
    if (embedOpen && embedWrap && !embedWrap.contains(e.target as Node)) embedOpen = false
  }}
  onbeforeunload={(e) => {
    if (doc.dirty) e.preventDefault()
  }}
/>

{#if doc.status === 'not-a-drawing'}
  <AttachmentView {path} />
{:else if doc.status === 'missing'}
  <EmptyState title={s.missingTitle} text={strings.note.notFoundText(name)} />
{:else if doc.status === 'error'}
  <EmptyState title={s.errorTitle} text={strings.note.errorText}>
    <button type="button" class="button" onclick={() => void doc.load()}>{strings.note.retry}</button>
  </EmptyState>
{:else}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <section class="drawing-editor" aria-label={s.label(name)} onkeydown={key}>
    <header>
      <h1>{name}</h1>
      <span class="save" class:problem={doc.save === 'failed' || doc.save === 'conflict'} aria-live="polite">{saveText}</span>
      <span class="spacer"></span>
      {#if doc.save === 'conflict'}
        <button type="button" class="button" onclick={() => void doc.takeTheirs()}>{s.takeTheirs}</button>
        <button type="button" class="button" onclick={() => void doc.keepMine()}>{s.keepMine}</button>
      {/if}
      <button type="button" class="button" title={strings.links.open} aria-haspopup="dialog" onclick={() => (app.linkDialog = path)}>
        <Icon name="link" size={16} />
        <span class="label">{strings.links.open}</span>
      </button>
      <button type="button" class="button" title={s.download} aria-haspopup="menu" aria-expanded={downloadMenu !== null} onclick={openDownload}>
        <Icon name="download" size={16} />
        <span class="label">{s.download}</span>
      </button>
      {#if !readonly}
        <div class="embed-wrap" bind:this={embedWrap}>
          <button type="button" class="button" aria-expanded={embedOpen} title={s.embed.button} onclick={() => (embedOpen = !embedOpen)}>
            <Icon name="note" size={16} />
            <span class="label">{s.embed.button}</span>
          </button>
          {#if embedOpen}
            <div class="embed-help" role="dialog" aria-label={s.embed.title}>
              <p class="title">{s.embed.title}</p>
              <div class="code">
                <code>{embedCode}</code>
                <button type="button" class="button" onclick={() => void copyEmbed()}>
                  <Icon name="copy" size={15} />
                  {s.embed.copy}
                </button>
              </div>
              {#each [s.embed.how, s.embed.width, s.embed.insert] as text, i (i)}
                <!-- On one line: whitespace here would show between code and text. -->
                <p>{#each text.split(EMBED_SAMPLE) as part, j (j)}{#if j % 2}<code class="sample">{part}</code>{:else}{part}{/if}{/each}</p>
              {/each}
            </div>
          {/if}
        </div>
      {/if}
      {#if onedit}
        <button type="button" class="button" onclick={onedit}>
          <Icon name="pencil" size={16} />
          {s.edit}
        </button>
      {/if}
      <button type="button" class="button primary" onclick={done}>
        <Icon name={readonly ? 'close' : 'check'} size={16} />
        {doneLabel}
      </button>
    </header>

    <div class="workspace">
      <div class="toolbar" role="toolbar" aria-label={s.tools} aria-orientation="vertical">
        {#each tools as t (t.id)}
          {#if mayEdit || t.id === 'select' || t.id === 'hand'}
            <button
              type="button"
              class="tool"
              class:on={tool === t.id}
              title={t.label}
              aria-label={t.label}
              aria-pressed={tool === t.id}
              onclick={() => {
                tool = t.id
                if (t.id !== 'select') selected = new Set()
              }}
            >
              <Icon name={t.icon} size={19} />
            </button>
          {/if}
          {#if t.id === 'diamond' && mayEdit}
            <button
              type="button"
              class="tool more"
              class:on={isMoreShape(tool) && !shapesOpen}
              title={s.tool.more}
              aria-label={s.tool.more}
              aria-expanded={shapesOpen}
              onclick={() => (shapesOpen = !shapesOpen)}
            >
              <Icon name={shapesOpen ? 'chevron-down' : moreShape} size={19} />
            </button>
            {#if shapesOpen}
              <div class="more-shapes" role="group" aria-label={s.tool.more}>
                {#each MORE_SHAPES as m (m)}
                  <button
                    type="button"
                    class="tool"
                    class:on={tool === m}
                    title={s.tool[m]}
                    aria-label={s.tool[m]}
                    aria-pressed={tool === m}
                    onclick={() => {
                      tool = m
                      moreShape = m
                      selected = new Set()
                    }}
                  >
                    <Icon name={m} size={19} />
                  </button>
                {/each}
              </div>
            {/if}
          {/if}
        {/each}
        {#if mayEdit}
          <button type="button" class="tool" class:on={iconsOpen} title={s.tool.icons} aria-label={s.tool.icons} aria-pressed={iconsOpen} onclick={() => (iconsOpen = !iconsOpen)}>
            <Icon name="image" size={19} />
          </button>
          <span class="sep"></span>
          <button type="button" class="tool" title={s.undo} aria-label={s.undo} disabled={!doc.canUndo} onclick={() => doc.undo()}>
            <Icon name="undo" size={19} />
          </button>
          <button type="button" class="tool" title={s.redo} aria-label={s.redo} disabled={!doc.canRedo} onclick={() => doc.redo()}>
            <Icon name="redo" size={19} />
          </button>
          <span class="sep"></span>
          <button type="button" class="tool" class:on={propsOpen} title={s.properties} aria-label={s.properties} aria-pressed={propsOpen} onclick={() => (propsOpen = !propsOpen)}>
            <Icon name="panel" size={19} />
          </button>
        {/if}
      </div>

      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div
        class="stage tool-{tool}"
        class:grid
        class:panning
        class:space={spaceDown}
        bind:this={stage}
        tabindex="0"
        use:cssVars={stageVars}
      >
        {#if doc.status === 'loading'}
          <p class="loading">{s.loading}</p>
        {/if}
        <svg
          bind:this={svg}
          class="dn-drawing canvas"
          role="img"
          aria-label={s.canvas}
          onpointerdown={down}
          onpointermove={moveEvent}
          onpointerup={up}
          onpointercancel={(e) => {
            pointers.delete(e.pointerId)
            cancelGesture()
          }}
          ondblclick={dblclick}
        >
          <g transform="translate({pan.x} {pan.y}) scale({zoom})">
            <g bind:this={layerEl}></g>
            <g class="overlay">
              {#if hintBox}
                <rect class="hint" x={hintBox.x} y={hintBox.y} width={hintBox.w} height={hintBox.h} rx={6 / zoom} vector-effect="non-scaling-stroke" />
              {/if}
              {#each outlines as o, i (i)}
                <rect class="outline" x={o.x} y={o.y} width={o.w} height={o.h} vector-effect="non-scaling-stroke" />
              {/each}
              {#if frameBox && !(single && isLinear(single)) && !editing}
                <rect class="frame" x={frameBox.x} y={frameBox.y} width={frameBox.w} height={frameBox.h} vector-effect="non-scaling-stroke" />
                {#each handles as h (h)}
                  {@const hp = handlePoint(frame!, h)}
                  <rect class="handle h-{h}" x={hp[0] - hs / 2} y={hp[1] - hs / 2} width={hs} height={hs} rx={2 / zoom} vector-effect="non-scaling-stroke" />
                {/each}
              {/if}
              {#each lineEnds as pt, i (i)}
                <circle class="handle" cx={pt[0]} cy={pt[1]} r={hs / 2} vector-effect="non-scaling-stroke" />
              {/each}
              {#if marquee}
                <rect class="marquee" x={marquee.x} y={marquee.y} width={marquee.w} height={marquee.h} vector-effect="non-scaling-stroke" />
              {/if}
            </g>
          </g>
        </svg>

        {#if editing && editBox}
          <textarea
            class="text-edit"
            aria-label={s.textLabel}
            value={editText}
            spellcheck="true"
            use:cssVars={editBox}
            use:focusOnMount
            oninput={(e) => typed(e.currentTarget.value)}
            onkeydown={textKey}
            onblur={finishText}
          ></textarea>
        {/if}

        <div class="zoom">
          <button type="button" title={s.zoomOut} aria-label={s.zoomOut} onclick={() => zoomBy(1 / 1.2)}><Icon name="zoom-out" size={17} /></button>
          <button type="button" class="pct" title={s.zoomReset} aria-label={s.zoomReset} onclick={() => zoomBy(1 / zoom)}>{s.zoom(Math.round(zoom * 100))}</button>
          <button type="button" title={s.zoomIn} aria-label={s.zoomIn} onclick={() => zoomBy(1.2)}><Icon name="zoom-in" size={17} /></button>
          <button type="button" title={s.zoomFit} aria-label={s.zoomFit} onclick={() => fitView()}><Icon name="fit" size={17} /></button>
        </div>
      </div>

      <!-- One panel at a time, so the canvas keeps its room. -->
      {#if mayEdit && iconsOpen}
        <IconLibrary onpick={insertPicture} onbuiltin={insertIcon} onclose={() => (iconsOpen = false)} />
      {:else if mayEdit && propsOpen}
        <DrawingProps
          style={panelStyle}
          keys={panelKeys}
          hasSelection={selection.length > 0}
          sketchy={scene.sketchy}
          {grid}
          onstyle={setStyle}
          onorder={setOrder}
          onduplicate={duplicate}
          ondelete={deleteSelection}
          onsketchy={setSketchy}
          ongrid={setGrid}
        />
      {/if}
    </div>
    {#if downloadMenu}
      <ContextMenu x={downloadMenu.x} y={downloadMenu.y} items={downloadMenu.items} onclose={() => (downloadMenu = null)} />
    {/if}
  </section>
{/if}

<style>
  .drawing-editor {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  header {
    display: flex;
    flex: none;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--line);
  }
  h1 {
    overflow: hidden;
    font-size: var(--text-md);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .save {
    flex: none;
    color: var(--text-faint);
    font-size: var(--text-xs);
  }
  .save.problem {
    color: var(--err);
  }
  .spacer {
    flex: 1;
  }
  .button {
    display: inline-flex;
    flex: none;
    align-items: center;
    gap: var(--space-1);
    height: 30px;
    padding: 0 var(--space-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    font-weight: 550;
  }
  .button:hover {
    background: var(--bg-hover);
  }
  .button.primary {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--on-accent);
  }
  .button.primary:hover {
    background: var(--accent-strong);
  }
  .embed-wrap {
    position: relative;
  }
  .embed-help {
    position: absolute;
    z-index: 20;
    top: calc(100% + 6px);
    right: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    width: min(360px, 86vw);
    padding: var(--space-3);
    border: 1px solid var(--line);
    border-radius: var(--radius);
    background: var(--bg-raised);
    box-shadow: var(--shadow);
    color: var(--text-muted);
    font-size: var(--text-sm);
  }
  .embed-help .title {
    color: var(--text);
    font-weight: 600;
  }
  .embed-help .sample {
    padding: 0 3px;
    border-radius: 4px;
    background: var(--code-bg);
    color: var(--text);
    font-size: 0.92em;
    white-space: nowrap;
  }
  .embed-help .code {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .embed-help code {
    flex: 1;
    min-width: 0;
    padding: 6px 8px;
    overflow-x: auto;
    border-radius: var(--radius-sm);
    background: var(--code-bg);
    color: var(--text);
    white-space: nowrap;
  }
  .workspace {
    display: flex;
    flex: 1;
    min-height: 0;
  }
  .toolbar {
    display: flex;
    flex: none;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-2);
    overflow: auto;
    border-right: 1px solid var(--line);
    background: var(--bg-raised);
  }
  .tool {
    display: grid;
    padding: 0;
    border: none;
    background: none;
    flex: none;
    place-items: center;
    width: 36px;
    height: 36px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
  }
  .tool:hover:not(:disabled) {
    background: var(--bg-hover);
    color: var(--text);
  }
  .tool.on {
    background: var(--accent-soft);
    color: var(--text);
  }
  .tool:disabled {
    opacity: 0.4;
  }
  /* A small corner mark: more tools behind this button. */
  .tool.more {
    position: relative;
  }
  .tool.more::after {
    position: absolute;
    right: 4px;
    bottom: 4px;
    border-right: 4px solid currentColor;
    border-top: 4px solid transparent;
    content: '';
    opacity: 0.7;
  }
  .tool.more[aria-expanded='true']::after {
    display: none;
  }
  .more-shapes {
    display: flex;
    flex: none;
    flex-direction: column;
    gap: 2px;
    border-radius: var(--radius-sm);
    background: var(--bg-hover);
  }
  .sep {
    flex: none;
    height: 1px;
    margin: var(--space-1) var(--space-1);
    background: var(--line);
  }
  .stage {
    position: relative;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    background: var(--bg);
    outline: none;
    touch-action: none;
    user-select: none;
  }
  .stage.grid {
    background-image: radial-gradient(circle, var(--line-strong) 1px, transparent 1.4px);
    background-position: var(--gx) var(--gy);
    background-size: var(--grid) var(--grid);
  }
  .canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    cursor: default;
  }
  .tool-rect .canvas,
  .tool-ellipse .canvas,
  .tool-diamond .canvas,
  .tool-triangle .canvas,
  .tool-hexagon .canvas,
  .tool-parallelogram .canvas,
  .tool-cylinder .canvas,
  .tool-star .canvas,
  .tool-cloud .canvas,
  .tool-line .canvas,
  .tool-arrow .canvas,
  .tool-pen .canvas {
    cursor: crosshair;
  }
  .tool-text .canvas {
    cursor: text;
  }
  .tool-hand .canvas,
  .space .canvas {
    cursor: grab;
  }
  .panning .canvas {
    cursor: grabbing;
  }
  .loading {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    color: var(--text-faint);
  }
  .overlay rect,
  .overlay circle {
    fill: none;
    stroke: var(--accent);
    stroke-width: 1;
  }
  .overlay .outline {
    stroke-dasharray: 4 3;
    stroke-opacity: 0.7;
  }
  .overlay .handle {
    fill: var(--bg);
    stroke-width: 1.5;
  }
  .h-nw,
  .h-se {
    cursor: nwse-resize;
  }
  .h-ne,
  .h-sw {
    cursor: nesw-resize;
  }
  .h-n,
  .h-s {
    cursor: ns-resize;
  }
  .h-e,
  .h-w {
    cursor: ew-resize;
  }
  .overlay .hint {
    stroke: var(--accent-2);
    stroke-width: 2;
  }
  .overlay .marquee {
    fill: var(--accent-soft);
    fill-opacity: 0.4;
    stroke-dasharray: 4 3;
  }
  .text-edit {
    position: absolute;
    top: var(--y);
    left: var(--x);
    width: var(--w);
    min-width: 20px;
    height: var(--h);
    padding: 0;
    overflow: hidden;
    border: none;
    outline: 1px dashed var(--accent);
    background: transparent;
    color: var(--ink);
    font-family: var(--family);
    font-size: var(--size) !important;
    line-height: 1.25;
    text-align: var(--align);
    white-space: pre;
    resize: none;
  }
  .zoom {
    position: absolute;
    right: var(--space-3);
    bottom: var(--space-3);
    display: flex;
    align-items: center;
    padding: 2px;
    border: 1px solid var(--line);
    border-radius: var(--radius);
    background: var(--bg-raised);
    box-shadow: var(--shadow);
  }
  .zoom button {
    display: grid;
    padding: 0 var(--space-1);
    border: none;
    background: none;
    place-items: center;
    min-width: 30px;
    height: 30px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  .zoom button:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .zoom .pct {
    min-width: 52px;
    font-variant-numeric: tabular-nums;
  }
  @media (max-width: 760px) {
    header {
      padding: var(--space-2) var(--space-3);
    }
    header .label {
      display: none;
    }
    .workspace {
      flex-direction: column;
    }
    .toolbar {
      flex-direction: row;
      scrollbar-width: thin;
      border-right: none;
      border-bottom: 1px solid var(--line);
    }
    .sep {
      width: 1px;
      height: auto;
      margin: var(--space-1) 2px;
    }
  }
</style>
