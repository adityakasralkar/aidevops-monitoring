import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { signedOut } from '../../store/authSlice'
import { useAppDispatch } from '../../store'

/**
 * Ends the session in the app: forgets the token, drops every cached query
 * (so the next user never sees this user's data) and goes to the login page.
 * The call to POST /auth/logout is added together with the API client.
 */
export function useLogout() {
  const dispatch = useAppDispatch()
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  return () => {
    dispatch(signedOut())
    queryClient.clear()
    void navigate({ to: '/login' })
  }
}
