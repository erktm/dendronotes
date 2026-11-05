// Helpers for vault-relative, "/"-separated paths. The rules for new names
// mirror the server (internal/vault/path.go) so problems show up while typing.

export const NOTE_EXTENSIONS = ['.md', '.markdown', '.txt']

export function basename(p: string): string {
  const i = p.lastIndexOf('/')
  return i < 0 ? p : p.slice(i + 1)
}

export function dirname(p: string): string {
  const i = p.lastIndexOf('/')
  return i < 0 ? '' : p.slice(0, i)
}

export function join(dir: string, name: string): string {
  return dir ? `${dir}/${name}` : name
}

export function extname(p: string): string {
  const b = basename(p)
  const i = b.lastIndexOf('.')
  return i <= 0 ? '' : b.slice(i).toLowerCase()
}

export function isNote(p: string): boolean {
  return NOTE_EXTENSIONS.includes(extname(p))
}

/** The name shown in the UI: Markdown notes without their ".md". */
export function displayName(p: string): string {
  const b = basename(p)
  return extname(b) === '.md' ? b.slice(0, -3) : b
}

/** Whether p is dir itself or lies below it. */
export function isInside(p: string, dir: string): boolean {
  return p === dir || p.startsWith(`${dir}/`)
}

/** All ancestor folders of p, outermost first. */
export function ancestors(p: string): string[] {
  const parts = p.split('/')
  return parts.slice(0, -1).map((_, i) => parts.slice(0, i + 1).join('/'))
}

export function encodePath(p: string): string {
  return p.split('/').map(encodeURIComponent).join('/')
}

// eslint-disable-next-line no-control-regex
const FORBIDDEN = /[\\/:*?"<>|\u0000-\u001f\u007f]/
const RESERVED = /^(con|prn|aux|nul|com[1-9]|lpt[1-9])$/i

/** Returns why a new file or folder name is not allowed, or null if it is fine. */
export function nameProblem(raw: string): string | null {
  const name = raw.normalize('NFC')
  if (!name.trim()) return 'Please enter a name.'
  if (name.startsWith('.')) return 'Names can’t start with a dot.'
  if (FORBIDDEN.test(name)) return 'Names can’t contain \\ / : * ? " < > |'
  if (/^\s|\s$/.test(name)) return 'Names can’t start or end with a space.'
  if (name.endsWith('.')) return 'Names can’t end with a dot.'
  if (RESERVED.test(name.split('.')[0] ?? '')) return 'That name is reserved on Windows.'
  if (new TextEncoder().encode(name).length > 255) return 'That name is too long.'
  return null
}

/** Adds ".md" unless the name already has a note extension. */
export function withNoteExtension(name: string): string {
  return isNote(name) ? name : `${name}.md`
}
