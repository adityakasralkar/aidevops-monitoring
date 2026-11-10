import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { api } from '../../lib/api'
import { useAppDispatch, useAppSelector } from '../../store'
import { signedOut } from '../../store/authSlice'

/**
 * Logs out: forgets the session in the app straight away, drops every cached
 * query (so the next user never sees this user's data), goes to the login
 * page, and ends the session on the server in the background.
 */
export function useLogout() {
  const dispatch = useAppDispatch()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const token = useAppSelector((state) => state.auth.token)

  return () => {
    if (token) {
      // Explicit token: a 401 here (already expired) is fine and ignored.
      api('/auth/logout', { method: 'POST', token }).catch(() => {})
    }
    dispatch(signedOut())
    queryClient.clear()
    void navigate({ to: '/login' })
  }
}
