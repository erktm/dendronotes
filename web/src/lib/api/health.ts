export type ServerState = 'checking' | 'ready' | 'unavailable' | 'offline'

/** Queries /readyz and maps the answer to a display state. */
export async function checkReady(signal: AbortSignal): Promise<ServerState> {
  try {
    const res = await fetch('/readyz', { signal, cache: 'no-store', credentials: 'same-origin' })
    return res.ok ? 'ready' : 'unavailable'
  } catch {
    return signal.aborted ? 'checking' : 'offline'
  }
}
