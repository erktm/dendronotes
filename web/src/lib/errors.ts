import { ApiError } from './api/client'
import { strings } from './strings'

/** A human-readable message for an error from the API. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return strings.errors.byCode[err.code] ?? strings.errors.withCode(err.code)
  return strings.errors.generic
}
