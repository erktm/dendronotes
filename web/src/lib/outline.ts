import { maskCode } from './synapses'

export interface Heading {
  level: number
  text: string
  /** 0-based line in the note. */
  line: number
  /** Anchor id, the same one the reading view gives the heading. */
  slug: string
}

/** Builds URL-fragment ids for headings, numbering duplicates like GitHub. */
export function slugger(): (text: string) => string {
  const seen = new Map<string, number>()
  return (text: string) => {
    const base =
      text
        .normalize('NFKD')
        .toLowerCase()
        .replace(/[^\p{L}\p{N}\s-]/gu, '')
        .trim()
        .replace(/\s+/g, '-') || 'section'
    const n = seen.get(base) ?? 0
    seen.set(base, n + 1)
    return n === 0 ? base : `${base}-${n}`
  }
}

/** ATX headings (# …) of a note, ignoring code blocks. */
export function outline(src: string): Heading[] {
  const slug = slugger()
  const out: Heading[] = []
  maskCode(src)
    .split('\n')
    .forEach((line, i) => {
      const m = /^ {0,3}(#{1,6})[ \t]+(.+?)[ \t#]*$/.exec(line)
      if (m) {
        const text = m[2]!.trim()
        out.push({ level: m[1]!.length, text, line: i, slug: slug(text) })
      }
    })
  return out
}

/**
 * The part of a note under a heading (up to the next heading of the same or
 * a higher level), for ![[Note#Heading]]. The whole note if not found.
 */
export function section(src: string, heading: string): string {
  const want = heading.trim().toLowerCase()
  const headings = outline(src)
  const i = headings.findIndex((h) => h.text.toLowerCase() === want)
  if (i < 0) return src
  const h = headings[i]!
  const next = headings.slice(i + 1).find((n) => n.level <= h.level)
  const lines = src.split('\n')
  return lines.slice(h.line, next ? next.line : lines.length).join('\n')
}
