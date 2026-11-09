import { request } from './client'

export type TokenScope = 'read' | 'write'

/** A personal API token (never the token itself). */
export interface APIToken {
  id: number
  name: string
  scope: TokenScope
  createdAt: string
  expiresAt?: string
  lastUsedAt?: string
  lastUsedIp?: string
}

export const tokens = () => request<{ tokens: APIToken[] }>('GET', '/api/me/tokens').then((r) => r.tokens)
/** Creates a token; the answer is the only time it is shown. */
export const createToken = (name: string, scope: TokenScope, expiresInDays: number) =>
  request<{ token: string; info: APIToken }>('POST', '/api/me/tokens', { body: { name, scope, expiresInDays } })
export const revokeToken = (id: number) => request<{ id: number }>('DELETE', `/api/me/tokens/${id}`)
