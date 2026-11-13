// Files and whole folders dropped from the computer (Finder, Explorer). The
// browser hands out folders as FileSystemEntry trees, which have to be taken
// during the drop event and read afterwards.

/** The files and folders of a drop (call it in the drop handler itself). */
export function droppedEntries(dt: DataTransfer | null): FileSystemEntry[] {
  const out: FileSystemEntry[] = []
  for (const item of dt?.items ?? []) {
    if (item.kind !== 'file') continue
    const entry = item.webkitGetAsEntry?.()
    if (entry) out.push(entry)
  }
  return out
}

/** Whether a drop holds at least one folder. */
export const hasFolder = (entries: readonly FileSystemEntry[]) => entries.some((e) => e.isDirectory)

export interface DroppedFile {
  /** The folder it was in, relative to the drop ("" for a dropped file). */
  dir: string
  file: File
}

export interface Dropped {
  /** Folders to create, parents before their children. */
  folders: string[]
  files: DroppedFile[]
  /** More files than the limit were found; only that many are listed. */
  truncated: boolean
}

const isDir = (e: FileSystemEntry): e is FileSystemDirectoryEntry => e.isDirectory
const isFile = (e: FileSystemEntry): e is FileSystemFileEntry => e.isFile

/** Reads every entry of a folder (the browser hands them out in batches). */
async function children(dir: FileSystemDirectoryEntry): Promise<FileSystemEntry[]> {
  const reader = dir.createReader()
  const out: FileSystemEntry[] = []
  for (;;) {
    const batch = await new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject))
    if (batch.length === 0) return out
    out.push(...batch)
  }
}

const fileOf = (f: FileSystemFileEntry) => new Promise<File>((resolve, reject) => f.file(resolve, reject))

/**
 * Walks a drop: folders with their paths, files with their folder. Hidden
 * names (.DS_Store, .git …) are left out, like everywhere in a soma.
 */
export async function readDropped(entries: readonly FileSystemEntry[], maxFiles: number): Promise<Dropped> {
  const out: Dropped = { folders: [], files: [], truncated: false }
  const walk = async (list: readonly FileSystemEntry[], dir: string): Promise<void> => {
    for (const e of list) {
      if (e.name.startsWith('.')) continue
      if (out.files.length >= maxFiles) {
        out.truncated = true
        return
      }
      const path = dir ? `${dir}/${e.name}` : e.name
      if (isDir(e)) {
        out.folders.push(path)
        await walk(await children(e), path)
      } else if (isFile(e)) {
        try {
          out.files.push({ dir, file: await fileOf(e) })
        } catch {
          // Unreadable (permissions, gone meanwhile): skipped.
        }
      }
    }
  }
  await walk(entries, '')
  return out
}
