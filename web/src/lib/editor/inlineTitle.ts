// The note's title (its file name) above the text, as Obsidian shows it:
// part of the scrolling document, but not of the file. Editing it renames
// the note; the text itself never gets an extra heading.

import { StateEffect, StateField, type Extension } from '@codemirror/state'
import { Decoration, type DecorationSet, EditorView, WidgetType } from '@codemirror/view'

export interface TitleSpec {
  title: string
  editable: boolean
  label: string
  /** Rename to the new title; resolves false when it did not work. */
  rename: (title: string) => Promise<boolean>
}

/** Sets or replaces the title. */
export const setTitle = StateEffect.define<TitleSpec>()

class TitleWidget extends WidgetType {
  constructor(readonly spec: TitleSpec) {
    super()
  }

  override eq(other: TitleWidget): boolean {
    return other.spec.title === this.spec.title && other.spec.editable === this.spec.editable && other.spec.label === this.spec.label
  }

  toDOM(view: EditorView): HTMLElement {
    const wrap = document.createElement('div')
    wrap.className = 'note-title cm-note-title'
    if (!this.spec.editable) {
      const h = document.createElement('h1')
      h.className = 'note-title-text'
      h.textContent = this.spec.title
      wrap.append(h)
      return wrap
    }
    const input = document.createElement('input')
    input.className = 'note-title-text'
    input.type = 'text'
    input.value = this.spec.title
    input.spellcheck = false
    input.setAttribute('aria-label', this.spec.label)
    input.setAttribute('autocomplete', 'off')
    const spec = this.spec
    let busy = false
    const commit = async (thenEdit: boolean) => {
      const next = input.value.trim()
      if (!next || next === spec.title) {
        input.value = spec.title
      } else if (!busy) {
        busy = true
        const ok = await spec.rename(next)
        busy = false
        if (!ok) input.value = spec.title
      }
      if (thenEdit) {
        view.focus()
        view.dispatch({ selection: { anchor: 0 }, scrollIntoView: true })
      }
    }
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === 'ArrowDown') {
        e.preventDefault()
        void commit(true)
      } else if (e.key === 'Escape') {
        e.preventDefault()
        input.value = spec.title
        view.focus()
      }
    })
    input.addEventListener('blur', () => void commit(false))
    wrap.append(input)
    return wrap
  }

  // The input handles its own keys and clicks; the editor stays out of it.
  override ignoreEvent(): boolean {
    return true
  }
}

function titleDecorations(spec: TitleSpec | null): DecorationSet {
  if (!spec) return Decoration.none
  return Decoration.set([Decoration.widget({ widget: new TitleWidget(spec), side: -1, block: true }).range(0)])
}

/** The title above the first line; set it with setTitle. */
export function inlineTitle(): Extension {
  return StateField.define<{ spec: TitleSpec | null; deco: DecorationSet }>({
    create: () => ({ spec: null, deco: Decoration.none }),
    update(value, tr) {
      for (const e of tr.effects) if (e.is(setTitle)) return { spec: e.value, deco: titleDecorations(e.value) }
      return tr.docChanged ? { spec: value.spec, deco: value.deco.map(tr.changes) } : value
    },
    provide: (f) => EditorView.decorations.from(f, (v) => v.deco),
  })
}
