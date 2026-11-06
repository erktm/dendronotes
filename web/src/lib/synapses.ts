// Synapses are connections between two notes (see CLAUDE.md, "What is a
// synapse"): wiki links [[Note]], [[Note|Alias]], [[Note#Heading]], embeds
// ![[Note]] and Markdown links to notes [Text](Other.md). Links to
// attachments, external URLs and headings in the same note are not synapses.

import { basename, dirname, extname, isNote } from './paths'

export interface SynapseRef {
  /** Note name or soma-relative path as written, without #heading and |alias. */
  target: string
  heading?: string
  alias?: string
  /** ![[Note]] instead of [[Note]]. */
  embed: boolean
  /** Written as a Markdown link [Text](path.md) instead of a wiki link. */
  markdown: boolean
  /** Offsets of the whole construct in the source. */
  from: number
  to: number
}

/** File types that are attachments, never notes. */
const ATTACHMENT_EXTENSIONS = new Set([
  '.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif', '.svg', '.bmp', '.ico',
  '.pdf', '.mp3', '.wav', '.ogg', '.m4a', '.flac', '.mp4', '.webm', '.mov', '.mkv',
  '.zip', '.csv', '.json', '.html', '.css', '.js',
])

/** Whether a wiki link target names a note (not an attachment). */
export function isNoteTarget(target: string): boolean {
  return !ATTACHMENT_EXTENSIONS.has(extname(target))
}

/**
 * Replaces fenced code blocks and inline code spans with spaces, keeping
 * every offset, so that brackets inside code are never read as synapses.
 */
export function maskCode(src: string): string {
  const lines = src.split('\n')
  let fence: { char: string; len: number } | null = null
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i] ?? ''
    const m = /^ {0,3}(`{3,}|~{3,})/.exec(line)
    if (fence) {
      if (m && m[1]![0] === fence.char && m[1]!.length >= fence.len && /^ {0,3}[`~]+\s*$/.test(line)) fence = null
      lines[i] = ' '.repeat(line.length)
      continue
    }
    if (m && !(m[1]![0] === '`' && line.slice(m.index + m[0].length).includes('`'))) {
      fence = { char: m[1]![0]!, len: m[1]!.length }
      lines[i] = ' '.repeat(line.length)
      continue
    }
    lines[i] = line.replace(/(`+)(.+?)\1(?!`)/g, (s) => ' '.repeat(s.length))
  }
  return lines.join('\n')
}

const WIKI = /(!?)\[\[([^[\]\n]+?)\]\]/g
const MD_LINK = /(!?)\[([^\]\n]*)\]\(\s*(<[^<>\n]+>|[^)\s]+)(?:\s+"[^"\n]*")?\s*\)/g
const SCHEME = /^[a-z][a-z0-9+.-]*:/i

/** Splits the inside of [[…]] into target, heading and alias. */
export function splitWikiTarget(inner: string): { target: string; heading?: string; alias?: string } {
  let rest = inner
  let alias: string | undefined
  const pipe = rest.indexOf('|')
  if (pipe >= 0) {
    alias = rest.slice(pipe + 1).trim() || undefined
    rest = rest.slice(0, pipe)
  }
  let heading: string | undefined
  const hash = rest.indexOf('#')
  if (hash >= 0) {
    heading = rest.slice(hash + 1).trim() || undefined
    rest = rest.slice(0, hash)
  }
  return { target: rest.trim(), heading, alias }
}

/** All synapses in a note, in source order. */
export function parseSynapses(src: string): SynapseRef[] {
  const masked = maskCode(src)
  const out: SynapseRef[] = []

  for (const m of masked.matchAll(WIKI)) {
    const { target, heading, alias } = splitWikiTarget(m[2]!)
    if (!target || !isNoteTarget(target)) continue // same-note heading or attachment
    out.push({ target, heading, alias, embed: m[1] === '!', markdown: false, from: m.index!, to: m.index! + m[0].length })
  }

  for (const m of masked.matchAll(MD_LINK)) {
    let href = m[3]!
    if (href.startsWith('<')) {
      if (href.length < 2 || !href.endsWith('>')) continue // unclosed <…: not a link destination
      href = href.slice(1, -1)
    }
    if (SCHEME.test(href) || href.startsWith('#') || href.startsWith('//')) continue
    const hash = href.indexOf('#')
    const heading = hash >= 0 ? decodeSafe(href.slice(hash + 1)) || undefined : undefined
    const path = decodeSafe(hash >= 0 ? href.slice(0, hash) : href)
    if (!path || !isNote(path)) continue
    out.push({ target: path, heading, alias: m[2] || undefined, embed: m[1] === '!', markdown: true, from: m.index!, to: m.index! + m[0].length })
  }

  return out.sort((a, b) => a.from - b.from)
}

function decodeSafe(s: string): string {
  try {
    return decodeURIComponent(s)
  } catch {
    return s
  }
}

/** Resolves "." and ".." segments; null if the path would leave the soma. */
export function normalizePath(p: string): string | null {
  const out: string[] = []
  for (const seg of p.split('/')) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') {
      if (out.length === 0) return null
      out.pop()
    } else {
      out.push(seg)
    }
  }
  return out.join('/')
}

const fold = (s: string) => s.normalize('NFC').toLowerCase()

function withNoteExt(t: string): string {
  return isNote(t) ? t : `${t}.md`
}

/**
 * Finds the note a synapse points to: a Markdown link is
 * relative to the current note; a wiki link matches by path or by file name
 * (case-insensitive). Several candidates: same folder first, then the
 * shortest path. Returns null for an unresolved synapse.
 */
export function resolveSynapse(
  ref: Pick<SynapseRef, 'target' | 'markdown'>,
  fromPath: string,
  notes: readonly string[],
): string | null {
  return resolveIn(ref, fromPath, notes, true)
}

/**
 * Finds an attachment (![[photo.png]], ![](photo.png)) among all files,
 * with the same rules as notes but without adding ".md".
 */
export function resolveFile(
  ref: Pick<SynapseRef, 'target' | 'markdown'>,
  fromPath: string,
  files: readonly string[],
): string | null {
  return resolveIn(ref, fromPath, files, false)
}

function resolveIn(
  ref: Pick<SynapseRef, 'target' | 'markdown'>,
  fromPath: string,
  notes: readonly string[],
  noteExt: boolean,
): string | null {
  const byFold = new Map<string, string>()
  for (const n of notes) byFold.set(fold(n), n)
  const exact = (p: string | null) => (p === null ? null : notes.includes(p) ? p : (byFold.get(fold(p)) ?? null))
  const here = dirname(fromPath)

  if (ref.markdown) {
    return exact(normalizePath(`${here}/${ref.target}`)) ?? exact(normalizePath(ref.target))
  }

  const bare = ref.target.replace(/^\/+/, '')
  const target = noteExt ? withNoteExt(bare) : bare
  let candidates: string[]
  if (target.includes('/')) {
    const direct = exact(normalizePath(target)) ?? exact(normalizePath(`${here}/${target}`))
    if (direct) return direct
    const suffix = `/${fold(target)}`
    candidates = notes.filter((n) => fold(n).endsWith(suffix))
  } else {
    const name = fold(target)
    candidates = notes.filter((n) => fold(basename(n)) === name)
  }
  if (candidates.length <= 1) return candidates[0] ?? null
  return [...candidates].sort(
    (a, b) =>
      Number(dirname(b) === here) - Number(dirname(a) === here) ||
      a.split('/').length - b.split('/').length ||
      (a < b ? -1 : a > b ? 1 : 0), // plain order, as on the server
  )[0]!
}

/** Where a note for an unresolved synapse is created. */
export function newNotePath(target: string, fromPath: string): string | null {
  const t = withNoteExt(target.replace(/^\/+/, ''))
  return t.includes('/') ? normalizePath(t) : normalizePath(`${dirname(fromPath)}/${t}`)
}

/** Text shown for a synapse. */
export function synapseLabel(ref: Pick<SynapseRef, 'target' | 'heading' | 'alias'>): string {
  if (ref.alias) return ref.alias
  const name = basename(ref.target).replace(/\.md$/i, '')
  return ref.heading ? `${name} › ${ref.heading}` : name
}
