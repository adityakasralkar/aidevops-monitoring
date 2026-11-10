import { ApiError } from './api'

/**
 * One short sentence for a failed request. `known` maps the server's error
 * codes to friendlier wording for a specific screen.
 */
export function apiErrorMessage(err: unknown, known: Record<string, string> = {}): string {
  if (!(err instanceof ApiError)) return 'Something went wrong. Try again.'
  if (known[err.code]) return known[err.code]
  switch (err.code) {
    case 'too_many_requests':
      return `Too many attempts. Try again in ${waitText(err.retryAfter)}.`
    case 'validation_failed':
    case 'network_error':
      return sentence(err.message)
    default:
      return 'Something went wrong. Try again.'
  }
}

function waitText(seconds = 60): string {
  if (seconds < 90) return `${seconds} seconds`
  return `${Math.ceil(seconds / 60)} minutes`
}

/** Server messages start lower-case without a full stop; make them a sentence. */
function sentence(text: string): string {
  const s = text.charAt(0).toUpperCase() + text.slice(1)
  return /[.!?]$/.test(s) ? s : `${s}.`
}
