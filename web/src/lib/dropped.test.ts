import { describe, expect, it } from 'vitest'
import { readDropped } from './dropped'

// Just enough of FileSystemEntry: folders hand out their entries in batches.
function file(name: string): FileSystemEntry {
  return { name, isFile: true, isDirectory: false, file: (ok: (f: File) => void) => ok(new File(['x'], name)) } as unknown as FileSystemEntry
}
function folder(name: string, entries: FileSystemEntry[]): FileSystemEntry {
  return {
    name,
    isFile: false,
    isDirectory: true,
    createReader: () => {
      const batches = [entries.slice(0, 2), entries.slice(2), []]
      return { readEntries: (ok: (e: FileSystemEntry[]) => void) => ok(batches.shift() ?? []) }
    },
  } as unknown as FileSystemEntry
}

describe('dropped folders', () => {
  const drop = [
    folder('Garden', [file('Plan.md'), file('.DS_Store'), folder('Photos', [file('bed.png'), file('pond.jpg')]), folder('.git', [file('HEAD')]), file('Ideas.md')]),
    file('Loose.md'),
  ]

  it('list folders before their files and leave hidden names out', async () => {
    const d = await readDropped(drop, 100)
    expect(d.folders).toEqual(['Garden', 'Garden/Photos'])
    expect(d.files.map((f) => `${f.dir}|${f.file.name}`)).toEqual([
      'Garden|Plan.md',
      'Garden/Photos|bed.png',
      'Garden/Photos|pond.jpg',
      'Garden|Ideas.md',
      '|Loose.md',
    ])
    expect(d.truncated).toBe(false)
  })

  it('stop at the limit', async () => {
    const d = await readDropped(drop, 2)
    expect(d.files).toHaveLength(2)
    expect(d.truncated).toBe(true)
  })
})
