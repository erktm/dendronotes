// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { elementNode } from './dom'
import { approxMeasure } from './render'
import { type ImageEl, add, emptyScene } from './scene'

const PNG = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=='

function picture(mono: boolean): ImageEl {
  return { id: 'pic', type: 'image', file: 'f', mono, x: 10, y: 20, w: 30, h: 30, stroke: 'green', width: 1, dash: 'solid', seed: 1 }
}

describe('pictures in the app', () => {
  it('show as they are, or as their shape in the color of the element', () => {
    const scene = (el: ImageEl) => add({ ...emptyScene(), files: { f: { src: PNG, w: 1, h: 1 } } }, el)
    const plain = elementNode(picture(false), scene(picture(false)), approxMeasure)
    expect(plain.querySelector('image')?.getAttribute('href')).toBe(PNG)
    expect(plain.querySelector('mask')).toBeNull()

    const node = (el: ImageEl) => elementNode(el, scene(el), approxMeasure)
    const [a, b] = [node(picture(true)), node(picture(true))]
    const mask = a.querySelector('mask')!
    expect(mask.getAttribute('mask-type')).toBe('alpha')
    expect(mask.querySelector('image')?.getAttribute('href')).toBe(PNG)
    const rect = a.querySelector('rect')!
    expect(rect.getAttribute('class')).toBe('t-green')
    expect(rect.getAttribute('mask')).toBe(`url(#${mask.id})`)
    // Two drawings on one page never share a mask.
    expect(b.querySelector('mask')!.id).not.toBe(mask.id)
  })
})
