// Roles in a soma and what they allow – the same order as store.RoleRank on
// the server, which enforces it anyway. Each role includes the ones below.

export type SomaRole = 'owner' | 'editor' | 'contributor' | 'writer' | 'viewer'
/** Roles a soma can be shared with (owner belongs to one account). */
export type ShareRole = Exclude<SomaRole, 'owner'>

const ORDER: readonly SomaRole[] = ['viewer', 'writer', 'contributor', 'editor', 'owner']

/** Share roles from least to most. */
export const SHARE_ROLES: readonly ShareRole[] = ['viewer', 'writer', 'contributor', 'editor']

export function rank(role: SomaRole | null | undefined): number {
  return role ? ORDER.indexOf(role) + 1 : 0
}

export interface Capabilities {
  /** Change existing notes (and tick tasks). */
  write: boolean
  /** Create notes, folders and files (also by following an unresolved synapse). */
  create: boolean
  /** Rename, move and delete. */
  organize: boolean
  /** Share the soma, rename or delete it. */
  manage: boolean
}

export type Capability = keyof Capabilities

export function capabilities(role: SomaRole | null | undefined): Capabilities {
  const r = rank(role)
  return { write: r >= 2, create: r >= 3, organize: r >= 4, manage: r >= 5 }
}
