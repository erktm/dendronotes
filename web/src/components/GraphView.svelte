<script lang="ts">
  import { untrack } from 'svelte'
  import {
    forceCollide,
    forceLink,
    forceManyBody,
    forceSimulation,
    forceX,
    forceY,
    type Simulation,
    type SimulationLinkDatum,
    type SimulationNodeDatum,
  } from 'd3-force'
  import Icon from './Icon.svelte'
  import { app } from '../lib/state/app.svelte'
  import { live } from '../lib/state/live.svelte'
  import { settings } from '../lib/state/settings.svelte'
  import * as api from '../lib/api/soma'
  import type { GraphData } from '../lib/api/soma'
  import { nodeRadius, selectGraph, type ViewNode } from '../lib/graph'
  import { displayName } from '../lib/paths'
  import { strings } from '../lib/strings'

  type Node = ViewNode & SimulationNodeDatum
  type Edge = SimulationLinkDatum<Node>

  /** The note the local graph is centered on (the one opened last). */
  let { center }: { center: string | null } = $props()

  let canvas: HTMLCanvasElement | undefined = $state()
  let data = $state.raw<GraphData | null>(null)
  let loaded = $state(false)

  // Load the graph, and again whenever the soma changes.
  $effect(() => {
    void live.version
    const ctrl = new AbortController()
    const timer = setTimeout(
      async () => {
        try {
          data = await api.graph(ctrl.signal)
        } catch {
          // Shown as empty; the next change event tries again.
        }
        loaded = true
      },
      untrack(() => loaded) ? 400 : 0,
    )
    return () => {
      clearTimeout(timer)
      ctrl.abort()
    }
  })

  const local = $derived(settings.graphMode === 'local' && center !== null)
  const selection = $derived(
    data
      ? selectGraph(
          data,
          { mode: settings.graphMode, depth: settings.graphDepth, missing: settings.graphMissing, orphans: settings.graphOrphans },
          center,
        )
      : null,
  )

  // --- Layout ------------------------------------------------------------

  let nodes: Node[] = []
  let edges: Edge[] = []
  let byId = new Map<string, Node>()
  let neighbors = new Map<string, Set<string>>()
  let sim: Simulation<Node, Edge> | null = null
  /** Screen position = world position × k + (x, y). */
  let view = { k: 1, x: 0, y: 0 }
  let hover: string | null = null
  let lastLayoutKey = ''

  $effect(() => {
    const sel = selection
    const key = `${settings.graphMode}|${settings.graphDepth}|${center}`
    if (!sel || !canvas) return
    untrack(() => {
      const refit = key !== lastLayoutKey
      lastLayoutKey = key
      rebuild(sel.nodes, sel.edges, refit)
    })
  })

  function rebuild(next: ViewNode[], nextEdges: { source: string; target: string }[], refit: boolean) {
    const old = byId
    nodes = next.map((n) => {
      const o = old.get(n.id)
      return o ? { ...n, x: o.x, y: o.y, vx: o.vx, vy: o.vy } : { ...n }
    })
    byId = new Map(nodes.map((n) => [n.id, n]))
    edges = nextEdges.map((e) => ({ source: e.source, target: e.target }))
    neighbors = new Map()
    for (const e of nextEdges) {
      if (!neighbors.has(e.source)) neighbors.set(e.source, new Set())
      if (!neighbors.has(e.target)) neighbors.set(e.target, new Set())
      neighbors.get(e.source)!.add(e.target)
      neighbors.get(e.target)!.add(e.source)
    }
    if (hover && !byId.has(hover)) hover = null

    const fresh = nodes.some((n) => n.x === undefined)
    sim?.stop()
    sim = forceSimulation<Node, Edge>(nodes)
      .force('link', forceLink<Node, Edge>(edges).id((d) => d.id).distance(72).strength(0.45))
      .force('charge', forceManyBody<Node>().strength(-240).distanceMax(600))
      // Notes without synapses stay close instead of drifting to the edge.
      .force('x', forceX<Node>(0).strength((d) => (d.degree === 0 ? 0.18 : 0.06)))
      .force('y', forceY<Node>(0).strength((d) => (d.degree === 0 ? 0.18 : 0.06)))
      .force('collide', forceCollide<Node>((d) => nodeRadius(d.degree) + 8))
      .on('tick', draw)
      .stop()

    if (refit || old.size === 0) {
      // Settle most of the layout before the first frame, then fit it.
      const ticks = Math.min(220, Math.ceil(200_000 / Math.max(nodes.length, 1)))
      for (let i = 0; i < ticks; i++) sim.tick()
      fit()
      sim.alpha(0.05).restart()
    } else {
      sim.alpha(fresh ? 0.4 : 0.1).restart()
    }
    draw()
  }

  // --- Drawing -------------------------------------------------------------

  interface Palette {
    accent: string
    accentStrong: string
    /** Notes that do not exist yet (unresolved synapses). */
    accent2: string
    line: string
    text: string
    muted: string
    faint: string
    bg: string
    font: string
  }
  let palette: Palette | null = null

  function readPalette(el: HTMLElement): Palette {
    const cs = getComputedStyle(el)
    const v = (name: string) => cs.getPropertyValue(name).trim()
    return {
      accent: v('--accent'),
      accentStrong: v('--accent-strong'),
      accent2: v('--accent-2'),
      line: v('--line-strong'),
      text: v('--text'),
      muted: v('--text-muted'),
      faint: v('--text-faint'),
      bg: v('--bg'),
      font: v('--font-ui') || 'sans-serif',
    }
  }

  // Colors follow the theme.
  $effect(() => {
    void settings.theme
    void settings.palette
    const dark = window.matchMedia('(prefers-color-scheme: dark)')
    const update = () => {
      if (!canvas) return
      palette = readPalette(canvas)
      draw()
    }
    update()
    dark.addEventListener('change', update)
    return () => dark.removeEventListener('change', update)
  })

  function draw() {
    const c = canvas
    if (!c) return
    const ctx = c.getContext('2d')
    if (!ctx) return
    const p = (palette ??= readPalette(c))
    const dpr = window.devicePixelRatio || 1
    const w = c.clientWidth
    const h = c.clientHeight
    if (c.width !== Math.round(w * dpr) || c.height !== Math.round(h * dpr)) {
      c.width = Math.round(w * dpr)
      c.height = Math.round(h * dpr)
    }
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, w, h)
    ctx.translate(view.x, view.y)
    ctx.scale(view.k, view.k)

    const focus = hover
    const near = focus ? neighbors.get(focus) : undefined
    const lit = (id: string) => !focus || id === focus || !!near?.has(id)
    const px = 1 / view.k

    // Synapses
    for (const e of edges) {
      const s = e.source as Node
      const t = e.target as Node
      if (s.x === undefined || t.x === undefined) continue
      const on = focus !== null && (s.id === focus || t.id === focus)
      ctx.globalAlpha = focus && !on ? 0.15 : on ? 1 : 0.85
      ctx.strokeStyle = on ? p.accent : p.line
      ctx.lineWidth = (on ? 1.6 : 1) * px
      ctx.setLineDash(t.missing ? [3 * px, 3 * px] : [])
      ctx.beginPath()
      ctx.moveTo(s.x, s.y!)
      ctx.lineTo(t.x, t.y!)
      ctx.stroke()
    }
    ctx.setLineDash([])

    // Notes
    for (const n of nodes) {
      if (n.x === undefined) continue
      const r = nodeRadius(n.degree)
      ctx.globalAlpha = lit(n.id) ? 1 : 0.22
      ctx.beginPath()
      ctx.arc(n.x, n.y!, r, 0, Math.PI * 2)
      if (n.missing) {
        ctx.fillStyle = p.bg
        ctx.fill()
        ctx.setLineDash([2 * px, 2 * px])
        ctx.strokeStyle = p.accent2
        ctx.lineWidth = 1.2 * px
        ctx.stroke()
        ctx.setLineDash([])
      } else {
        ctx.fillStyle = n.id === center || n.id === focus ? p.accentStrong : p.accent
        ctx.fill()
      }
      if (n.id === center) {
        ctx.beginPath()
        ctx.arc(n.x, n.y!, r + 3.5 * px + 2, 0, Math.PI * 2)
        ctx.strokeStyle = p.accent
        ctx.lineWidth = 1.5 * px
        ctx.stroke()
      }
    }

    // Names: all once zoomed in far enough for them to fit, else the focus,
    // its neighbors and the open note.
    const all = view.k >= 1.1 || nodes.length <= 12
    ctx.font = `${12 * px}px ${p.font}`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    for (const n of nodes) {
      if (n.x === undefined) continue
      const show = all || n.id === focus || n.id === center || !!near?.has(n.id)
      if (!show) continue
      ctx.globalAlpha = lit(n.id) ? 1 : 0.25
      ctx.fillStyle = n.id === focus || n.id === center ? p.text : n.missing ? p.faint : p.muted
      ctx.fillText(n.label, n.x, n.y! + nodeRadius(n.degree) + 4 * px)
    }
    ctx.globalAlpha = 1
  }

  // --- View ----------------------------------------------------------------

  function fit() {
    const c = canvas
    if (!c || nodes.length === 0) return
    let [x0, y0, x1, y1] = [Infinity, Infinity, -Infinity, -Infinity]
    for (const n of nodes) {
      x0 = Math.min(x0, n.x ?? 0)
      y0 = Math.min(y0, n.y ?? 0)
      x1 = Math.max(x1, n.x ?? 0)
      y1 = Math.max(y1, n.y ?? 0)
    }
    const w = c.clientWidth
    const h = c.clientHeight
    const k = Math.min(w / (x1 - x0 + 120), h / (y1 - y0 + 120), 2.2)
    view = { k, x: w / 2 - ((x0 + x1) / 2) * k, y: h / 2 - ((y0 + y1) / 2) * k }
    draw()
  }

  function zoomBy(factor: number, sx?: number, sy?: number) {
    const c = canvas
    if (!c) return
    const x = sx ?? c.clientWidth / 2
    const y = sy ?? c.clientHeight / 2
    const k = Math.min(Math.max(view.k * factor, 0.08), 6)
    const f = k / view.k
    view = { k, x: x - (x - view.x) * f, y: y - (y - view.y) * f }
    draw()
  }

  function toWorld(e: PointerEvent | WheelEvent): [number, number] {
    const r = canvas!.getBoundingClientRect()
    return [(e.clientX - r.left - view.x) / view.k, (e.clientY - r.top - view.y) / view.k]
  }

  function nodeAt(x: number, y: number): Node | null {
    let best: Node | null = null
    let bestD = Infinity
    for (const n of nodes) {
      const d = Math.hypot((n.x ?? 0) - x, (n.y ?? 0) - y)
      const reach = nodeRadius(n.degree) + 4 / view.k
      if (d <= reach && d < bestD) {
        best = n
        bestD = d
      }
    }
    return best
  }

  // --- Interaction -----------------------------------------------------------

  let drag: { node: Node | null; startX: number; startY: number; moved: boolean; viewX: number; viewY: number } | null = null

  function onpointerdown(e: PointerEvent) {
    if (e.button !== 0) return
    const [x, y] = toWorld(e)
    const node = nodeAt(x, y)
    drag = { node, startX: e.clientX, startY: e.clientY, moved: false, viewX: view.x, viewY: view.y }
    canvas!.setPointerCapture(e.pointerId)
    if (node) {
      node.fx = node.x
      node.fy = node.y
    }
  }

  function onpointermove(e: PointerEvent) {
    if (drag) {
      const dx = e.clientX - drag.startX
      const dy = e.clientY - drag.startY
      if (!drag.moved && Math.hypot(dx, dy) > 3) {
        drag.moved = true
        if (drag.node) sim?.alphaTarget(0.25).restart()
      }
      if (!drag.moved) return
      if (drag.node) {
        const [x, y] = toWorld(e)
        drag.node.fx = x
        drag.node.fy = y
      } else {
        view = { ...view, x: drag.viewX + dx, y: drag.viewY + dy }
      }
      draw()
      return
    }
    const [x, y] = toWorld(e)
    const id = nodeAt(x, y)?.id ?? null
    if (id !== hover) {
      hover = id
      canvas!.style.cursor = id ? 'pointer' : 'grab'
      draw()
    }
  }

  function onpointerup(e: PointerEvent) {
    const d = drag
    drag = null
    if (!d) return
    canvas!.releasePointerCapture(e.pointerId)
    if (d.node) {
      d.node.fx = null
      d.node.fy = null
      sim?.alphaTarget(0)
      if (!d.moved) open(d.node)
    }
  }

  function onwheel(e: WheelEvent) {
    e.preventDefault()
    const r = canvas!.getBoundingClientRect()
    zoomBy(Math.exp(-e.deltaY * (e.ctrlKey ? 0.01 : 0.0015)), e.clientX - r.left, e.clientY - r.top)
  }

  function open(n: ViewNode) {
    if (!n.missing) {
      void app.navigate(n.id)
      return
    }
    // A missing note is created where the first synapse to it would create it.
    const from = data?.edges.find((e) => e.target === n.id)?.source
    if (from) void app.openSynapse(n.label, false, from)
  }

  // Canvas size and wheel (the wheel listener must not be passive).
  $effect(() => {
    const c = canvas
    if (!c) return
    const ro = new ResizeObserver(() => draw())
    ro.observe(c)
    c.addEventListener('wheel', onwheel, { passive: false })
    return () => {
      ro.disconnect()
      c.removeEventListener('wheel', onwheel)
      sim?.stop()
    }
  })

  const centerName = $derived(center ? displayName(center) : '')
</script>

<section class="graph" aria-label={selection ? strings.graph.label(selection.nodes.length, selection.edges.length) : strings.graph.title}>
  {#if selection && selection.edges.length === 0 && !local}
    <!-- Behind the canvas and out of the way of notes without synapses. -->
    <p class="state backdrop" class:low={selection.nodes.length > 0}>{strings.graph.empty}</p>
  {/if}
  <!-- Drawn for the eye; the list below carries the same notes for assistive technology. -->
  <canvas
    bind:this={canvas}
    aria-hidden="true"
    {onpointerdown}
    {onpointermove}
    {onpointerup}
    onpointercancel={onpointerup}
    onpointerleave={() => {
      if (!drag && hover) {
        hover = null
        draw()
      }
    }}
  ></canvas>

  <div class="controls">
    <div class="segmented" role="group" aria-label={strings.graph.mode}>
      <button type="button" aria-pressed={!local} onclick={() => (settings.graphMode = 'global')}>{strings.graph.global}</button>
      <button
        type="button"
        aria-pressed={local}
        disabled={center === null}
        title={center === null ? strings.graph.noCenter : undefined}
        onclick={() => (settings.graphMode = 'local')}
      >
        {center ? strings.graph.localOf(centerName) : strings.graph.local}
      </button>
    </div>
    {#if local}
      <div class="segmented" role="group" aria-label={strings.graph.depth}>
        {#each [1, 2, 3] as d (d)}
          <button type="button" aria-pressed={settings.graphDepth === d} aria-label="{strings.graph.depth} {d}" onclick={() => (settings.graphDepth = d)}>
            {d}
          </button>
        {/each}
      </div>
    {/if}
    <label class="check"><input type="checkbox" bind:checked={settings.graphMissing} /> {strings.graph.missing}</label>
    {#if !local}
      <label class="check"><input type="checkbox" bind:checked={settings.graphOrphans} /> {strings.graph.orphans}</label>
    {/if}
  </div>

  <div class="zoom">
    <button type="button" aria-label={strings.graph.zoomIn} title={strings.graph.zoomIn} onclick={() => zoomBy(1.3)}><Icon name="plus" size={16} /></button>
    <button type="button" aria-label={strings.graph.zoomOut} title={strings.graph.zoomOut} onclick={() => zoomBy(1 / 1.3)}><Icon name="minus" size={16} /></button>
    <button type="button" aria-label={strings.graph.fit} title={strings.graph.fit} onclick={fit}><Icon name="fit" size={16} /></button>
  </div>

  <p class="foot">
    {#if selection}
      <span>{strings.graph.count(selection.nodes.length, selection.edges.length)}</span>
    {/if}
    <span class="hint">{strings.graph.hint}</span>
  </p>

  {#if !data && !loaded}
    <p class="state">{strings.graph.loading}</p>
  {/if}

  <!-- The same notes for screen readers and keyboards. -->
  {#if selection}
    <ul class="note-list" aria-label={strings.graph.listLabel}>
      {#each selection.nodes.slice(0, 500) as n (n.id)}
        <li>
          <button type="button" onclick={() => open(n)}>
            {n.label}{n.missing ? ` (${strings.graph.notCreated})` : ''}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .graph {
    position: relative;
    height: 100%;
    min-height: 0;
    overflow: hidden;
    background: var(--bg);
  }
  canvas {
    position: relative; /* above the backdrop hint */
    display: block;
    width: 100%;
    height: 100%;
    cursor: grab;
    touch-action: none;
  }
  .controls {
    position: absolute;
    top: 12px;
    left: 12px;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    max-width: calc(100% - 140px);
    padding: 6px;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--bg-raised);
    box-shadow: var(--shadow);
    font-size: var(--text-sm);
  }
  .segmented {
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--bg-sunken);
  }
  .segmented button {
    max-width: 220px;
    height: 26px;
    padding: 0 10px;
    overflow: hidden;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text-muted);
    font: inherit;
    font-size: 12.5px;
    font-weight: 550;
    white-space: nowrap;
    text-overflow: ellipsis;
    cursor: pointer;
  }
  .segmented button[aria-pressed='true'] {
    background: var(--bg-raised);
    color: var(--text);
    box-shadow: var(--shadow);
  }
  .segmented button:disabled {
    cursor: not-allowed;
    opacity: 0.5;
  }
  .check {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 0 4px;
    color: var(--text-muted);
    cursor: pointer;
  }
  .check input {
    accent-color: var(--accent);
  }
  .zoom {
    position: absolute;
    top: 12px;
    right: 12px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 4px;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--bg-raised);
    box-shadow: var(--shadow);
  }
  .zoom button {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }
  .zoom button:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .foot {
    position: absolute;
    right: 16px;
    bottom: 10px;
    left: 16px;
    display: flex;
    justify-content: space-between;
    gap: 16px;
    color: var(--text-faint);
    font-size: var(--text-xs);
    pointer-events: none;
  }
  .state {
    position: absolute;
    top: 50%;
    left: 50%;
    max-width: 360px;
    color: var(--text-muted);
    text-align: center;
    transform: translate(-50%, -50%);
  }
  .state.backdrop {
    color: var(--text-faint);
    font-size: var(--text-sm);
    pointer-events: none;
  }
  .state.backdrop.low {
    top: auto;
    bottom: 44px;
    transform: translateX(-50%);
  }
  /* Hidden for the eye, there for screen readers; shown when tabbed into. */
  .note-list {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: 0;
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    list-style: none;
    white-space: nowrap;
  }
  .note-list:focus-within {
    bottom: 36px;
    left: 12px;
    width: 260px;
    height: auto;
    max-height: 50%;
    padding: 6px;
    overflow: auto;
    clip-path: none;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--bg-raised);
    box-shadow: var(--shadow);
  }
  .note-list button {
    display: block;
    width: 100%;
    padding: 4px 8px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-sm);
    text-align: left;
    cursor: pointer;
  }
  .note-list button:focus-visible {
    background: var(--bg-hover);
    outline: 2px solid var(--focus);
  }
  @media (max-width: 760px) {
    .hint {
      display: none;
    }
  }
</style>
