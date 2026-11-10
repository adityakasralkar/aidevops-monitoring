import { useMutation } from '@tanstack/react-query'
import { api } from '../../lib/api'

export interface PasswordChange {
  current_password: string
  new_password: string
}

/**
 * Sets a new password. The server keeps this session and logs out every
 * other device. A wrong current password answers 403 (not 401), so it
 * never looks like an expired session.
 */
export function useChangePassword() {
  return useMutation({
    mutationFn: (input: PasswordChange) => api<void>('/auth/password', { method: 'POST', body: input }),
  })
}
