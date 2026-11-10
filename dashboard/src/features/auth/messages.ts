import { apiErrorMessage } from '../../lib/errors'

/** Turns a failed login / sign-up into one short sentence for the form. */
export function authErrorMessage(err: unknown): string {
  return apiErrorMessage(err, {
    invalid_credentials: 'Wrong email or password.',
    email_taken: 'An account with this email already exists. Log in instead.',
  })
}
