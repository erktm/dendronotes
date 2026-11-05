import { ApiError } from './api/client'

const messages: Record<string, string> = {
  network: 'Can’t reach the server. Check your connection.',
  invalid_name: 'That name isn’t allowed. Avoid \\ / : * ? " < > | and leading dots.',
  invalid_path: 'That path isn’t valid.',
  exists: 'Something with that name already exists here.',
  not_found: 'It no longer exists – it may have been moved or deleted.',
  move_into_self: 'A folder can’t be moved into itself.',
  read_only: 'This file is read-only.',
  too_large: 'This note is too large to edit here.',
  not_a_note: 'Only text notes (.md, .markdown, .txt) can be edited.',
  is_directory: 'That’s a folder, not a file.',
  not_a_directory: 'The target is not a folder.',
  cross_origin_request: 'The request was blocked as cross-origin.',
  rate_limited: 'Too many attempts. Please wait a moment.',
}

/** A human-readable message for an error from the API. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return messages[err.code] ?? `Something went wrong (${err.code}).`
  return 'Something went wrong.'
}
