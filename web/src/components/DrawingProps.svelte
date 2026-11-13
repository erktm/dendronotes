<script lang="ts">
  // The properties panel of the drawing editor: the look of the selected
  // elements (or of the next one drawn), their order, and the drawing's
  // own options.
  import Icon from './Icon.svelte'
  import { COLORS } from '../lib/drawing/colors'
  import { FONTS, FONT_SIZES, HEADS, STROKE_WIDTHS, type Order } from '../lib/drawing/scene'
  import type { Style, StyleKey } from '../lib/drawing/style'
  import { strings } from '../lib/strings'

  let {
    style,
    keys,
    hasSelection,
    sketchy,
    grid,
    onstyle,
    onorder,
    onduplicate,
    ondelete,
    onsketchy,
    ongrid,
  }: {
    /** What the panel shows: the first selected element's look, or the next element's. */
    style: Style
    /** What applies to the selection (or the tool). */
    keys: ReadonlySet<StyleKey>
    hasSelection: boolean
    sketchy: boolean
    grid: boolean
    onstyle: (patch: Partial<Style>) => void
    onorder: (order: Order) => void
    onduplicate: () => void
    ondelete: () => void
    onsketchy: (on: boolean) => void
    ongrid: (on: boolean) => void
  } = $props()

  const s = strings.drawing
</script>

<aside class="props" aria-label={s.properties}>
  {#if keys.has('stroke')}
    <fieldset>
      <legend>{s.stroke}</legend>
      <div class="swatches">
        {#each COLORS as c (c)}
          <button
            type="button"
            class="swatch line-{c}"
            class:on={style.stroke === c}
            title={s.color(c)}
            aria-label={s.color(c)}
            aria-pressed={style.stroke === c}
            onclick={() => onstyle({ stroke: c })}
          ></button>
        {/each}
      </div>
    </fieldset>
  {/if}

  {#if keys.has('fill')}
    <fieldset>
      <legend>{s.fill}</legend>
      <div class="swatches">
        <button
          type="button"
          class="swatch none"
          class:on={style.fill === 'none'}
          title={s.noFill}
          aria-label={s.noFill}
          aria-pressed={style.fill === 'none'}
          onclick={() => onstyle({ fill: 'none' })}
        ></button>
        {#each COLORS as c (c)}
          <button
            type="button"
            class="swatch tint-{c}"
            class:on={style.fill === c}
            title={s.color(c)}
            aria-label={`${s.fill}: ${s.color(c)}`}
            aria-pressed={style.fill === c}
            onclick={() => onstyle({ fill: c })}
          ></button>
        {/each}
      </div>
    </fieldset>
  {/if}

  {#if keys.has('width')}
    <fieldset>
      <legend>{s.width}</legend>
      <div class="row">
        {#each STROKE_WIDTHS as w (w)}
          <button type="button" class="opt" class:on={style.width === w} title={s.widths[w]} aria-label={s.widths[w]} aria-pressed={style.width === w} onclick={() => onstyle({ width: w })}>
            <svg width="24" height="24" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 12h16" stroke-width={w + 0.5} /></svg>
          </button>
        {/each}
      </div>
    </fieldset>
  {/if}

  {#if keys.has('dash')}
    <fieldset>
      <legend>{s.dash}</legend>
      <div class="row">
        {#each ['solid', 'dashed', 'dotted'] as const as d (d)}
          <button type="button" class="opt" class:on={style.dash === d} title={s.dashes[d]} aria-label={s.dashes[d]} aria-pressed={style.dash === d} onclick={() => onstyle({ dash: d })}>
            <svg width="24" height="24" viewBox="0 0 24 24" aria-hidden="true">
              <path d="M3 12h18" stroke-width="2" stroke-dasharray={d === 'dashed' ? '5 3' : d === 'dotted' ? '0 4' : 'none'} />
            </svg>
          </button>
        {/each}
        {#if keys.has('rounded')}
          <button type="button" class="opt" class:on={style.rounded} title={s.rounded} aria-label={s.rounded} aria-pressed={style.rounded} onclick={() => onstyle({ rounded: !style.rounded })}>
            <svg width="24" height="24" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 19V11a6 6 0 0 1 6-6h8" stroke-width="2" /></svg>
          </button>
        {/if}
        {#if keys.has('flip')}
          <button type="button" class="opt" class:on={style.flip} title={s.flip} aria-label={s.flip} aria-pressed={style.flip} onclick={() => onstyle({ flip: !style.flip })}>
            <svg width="24" height="24" viewBox="0 0 24 24" aria-hidden="true"><path d="M3.5 12h17M12 3.5l4.5 5h-9zM7.5 15.5h9l-4.5 5z" stroke-width="1.6" stroke-linejoin="round" /></svg>
          </button>
        {/if}
      </div>
    </fieldset>
  {/if}

  {#if keys.has('startHead')}
    <fieldset>
      <legend>{s.ends}</legend>
      <div class="selects">
        <label>
          <span>{s.startHead}</span>
          <select value={style.startHead} onchange={(e) => onstyle({ startHead: e.currentTarget.value as Style['startHead'] })}>
            {#each HEADS as h (h)}<option value={h}>{s.heads[h]}</option>{/each}
          </select>
        </label>
        <label>
          <span>{s.endHead}</span>
          <select value={style.endHead} onchange={(e) => onstyle({ endHead: e.currentTarget.value as Style['endHead'] })}>
            {#each HEADS as h (h)}<option value={h}>{s.heads[h]}</option>{/each}
          </select>
        </label>
      </div>
      <div class="row">
        <button type="button" class="opt wide" class:on={style.elbow} aria-pressed={style.elbow} onclick={() => onstyle({ elbow: !style.elbow })}>
          <Icon name="elbow" size={18} />
          {s.elbow}
        </button>
      </div>
    </fieldset>
  {/if}

  {#if keys.has('font')}
    <fieldset>
      <legend>{s.font}</legend>
      <div class="row">
        {#each FONTS as f (f)}
          <button type="button" class="opt font-{f}" class:on={style.font === f} aria-pressed={style.font === f} onclick={() => onstyle({ font: f })}>{s.fonts[f]}</button>
        {/each}
      </div>
      <div class="row">
        {#each FONT_SIZES as z (z)}
          <button type="button" class="opt" class:on={style.size === z} title={s.sizeNames[z]} aria-label={s.sizeNames[z]} aria-pressed={style.size === z} onclick={() => onstyle({ size: z })}>{s.sizes[z]}</button>
        {/each}
      </div>
      {#if keys.has('align')}
        <div class="row">
          {#each ['left', 'center', 'right'] as const as a (a)}
            <button type="button" class="opt" class:on={style.align === a} title={s.aligns[a]} aria-label={s.aligns[a]} aria-pressed={style.align === a} onclick={() => onstyle({ align: a })}>
              <svg width="24" height="24" viewBox="0 0 24 24" aria-hidden="true">
                <path d={a === 'left' ? 'M5 7h14M5 12h9M5 17h12' : a === 'right' ? 'M5 7h14M10 12h9M7 17h12' : 'M5 7h14M7.5 12h9M6 17h12'} stroke-width="1.7" />
              </svg>
            </button>
          {/each}
        </div>
      {/if}
    </fieldset>
  {/if}

  {#if hasSelection}
    <fieldset>
      <legend>{s.layers}</legend>
      <div class="row">
        <button type="button" class="opt" title={s.toBack} aria-label={s.toBack} onclick={() => onorder('back')}><Icon name="to-back" size={18} /></button>
        <button type="button" class="opt" title={s.backward} aria-label={s.backward} onclick={() => onorder('backward')}><Icon name="backward" size={18} /></button>
        <button type="button" class="opt" title={s.forward} aria-label={s.forward} onclick={() => onorder('forward')}><Icon name="forward" size={18} /></button>
        <button type="button" class="opt" title={s.toFront} aria-label={s.toFront} onclick={() => onorder('front')}><Icon name="to-front" size={18} /></button>
      </div>
      <div class="row">
        <button type="button" class="opt" title={s.duplicate} aria-label={s.duplicate} onclick={onduplicate}><Icon name="copy" size={18} /></button>
        <button type="button" class="opt danger" title={s.delete} aria-label={s.delete} onclick={ondelete}><Icon name="trash" size={18} /></button>
      </div>
    </fieldset>
  {/if}

  <fieldset>
    <legend>{s.drawingOptions}</legend>
    <label class="toggle">
      <input type="checkbox" checked={sketchy} onchange={(e) => onsketchy(e.currentTarget.checked)} />
      <Icon name="sketch" size={16} />
      {s.sketchy}
    </label>
    <label class="toggle">
      <input type="checkbox" checked={grid} onchange={(e) => ongrid(e.currentTarget.checked)} />
      <Icon name="grid" size={16} />
      {s.grid}
    </label>
  </fieldset>
</aside>

<style>
  .props {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    width: 212px;
    padding: var(--space-3);
    overflow: auto;
    border-left: 1px solid var(--line);
    background: var(--bg-raised);
  }
  fieldset {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
    padding: 0;
    border: none;
  }
  legend {
    margin-bottom: var(--space-1);
    padding: 0;
    color: var(--text-muted);
    font-size: var(--text-xs);
    font-weight: 600;
  }
  .swatches {
    display: grid;
    grid-template-columns: repeat(6, 26px);
    gap: 6px;
  }
  .swatch {
    padding: 0;
    width: 26px;
    height: 26px;
    border: 1px solid var(--line-strong);
    border-radius: 50%;
  }
  .swatch.on {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  .swatch.none {
    background: linear-gradient(135deg, transparent 45%, var(--err) 45% 55%, transparent 55%), var(--bg);
  }
  .line-ink { background: var(--text); }
  .line-gray { background: var(--draw-gray); }
  .line-red { background: var(--draw-red); }
  .line-orange { background: var(--draw-orange); }
  .line-yellow { background: var(--draw-yellow); }
  .line-green { background: var(--draw-green); }
  .line-teal { background: var(--draw-teal); }
  .line-blue { background: var(--draw-blue); }
  .line-violet { background: var(--draw-violet); }
  .line-pink { background: var(--draw-pink); }
  .tint-ink { background: var(--bg-sunken); }
  .tint-gray { background: var(--draw-gray-fill); }
  .tint-red { background: var(--draw-red-fill); }
  .tint-orange { background: var(--draw-orange-fill); }
  .tint-yellow { background: var(--draw-yellow-fill); }
  .tint-green { background: var(--draw-green-fill); }
  .tint-teal { background: var(--draw-teal-fill); }
  .tint-blue { background: var(--draw-blue-fill); }
  .tint-violet { background: var(--draw-violet-fill); }
  .tint-pink { background: var(--draw-pink-fill); }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }
  .opt {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-1);
    min-width: 34px;
    height: 32px;
    padding: 0 var(--space-2);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font-size: var(--text-sm);
  }
  .opt svg {
    fill: none;
    stroke: currentColor;
    stroke-linecap: round;
  }
  .opt:hover {
    background: var(--bg-hover);
  }
  .opt.on {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .opt.wide {
    flex: 1;
  }
  .opt.danger:hover {
    color: var(--err);
  }
  .font-sans {
    font-family: var(--font-ui);
  }
  .font-serif {
    font-family: var(--font-serif);
  }
  .font-mono {
    font-family: var(--font-mono);
  }
  .selects {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-2);
  }
  .selects label {
    display: flex;
    flex-direction: column;
    gap: 2px;
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  select {
    height: 30px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--text);
    font-size: var(--text-sm);
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  @media (max-width: 760px) {
    .props {
      flex-direction: row;
      width: auto;
      max-height: 40vh;
      overflow-x: auto;
      border-top: 1px solid var(--line);
      border-left: none;
    }
    fieldset {
      flex: none;
    }
  }
</style>
