import { ApiError } from '../../lib/api'

/** Turns a failed login / sign-up into one short sentence for the form. */
export function authErrorMessage(err: unknown): string {
  if (!(err instanceof ApiError)) return 'Something went wrong. Try again.'
  switch (err.code) {
    case 'invalid_credentials':
      return 'Wrong email or password.'
    case 'email_taken':
      return 'An account with this email already exists. Log in instead.'
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

function sentence(text: string): string {
  const s = text.charAt(0).toUpperCase() + text.slice(1)
  return /[.!?]$/.test(s) ? s : `${s}.`
}
