/** An error answer from the API (or a network failure with status 0). */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly body: Record<string, unknown>

  constructor(status: number, code: string, body: Record<string, unknown> = {}) {
    super(code)
    this.status = status
    this.code = code
    this.body = body
  }
}

let unauthorizedHandler: (() => void) | null = null

/** Registers a callback for 401 answers (session expired, logged out elsewhere). */
export function onUnauthorized(fn: () => void): void {
  unauthorizedHandler = fn
}

interface RequestOptions {
  body?: unknown
  headers?: Record<string, string>
  signal?: AbortSignal
  /** Do not trigger the global 401 handler (login, initial check). */
  expect401?: boolean
}

/** Sends a JSON request to the API and returns the decoded answer. */
export async function request<T>(method: string, url: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json', ...opts.headers }
  let body: string | undefined
  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(opts.body)
  }

  let res: Response
  try {
    res = await fetch(url, { method, headers, body, credentials: 'same-origin', cache: 'no-store', signal: opts.signal })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, 'network')
  }

  const text = await res.text()
  let data: unknown
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = undefined
    }
  }
  if (!res.ok) {
    const obj = data !== null && typeof data === 'object' ? (data as Record<string, unknown>) : {}
    const code = typeof obj.error === 'string' ? obj.error : `http_${res.status}`
    if (res.status === 401 && !opts.expect401) unauthorizedHandler?.()
    throw new ApiError(res.status, code, obj)
  }
  return data as T
}
