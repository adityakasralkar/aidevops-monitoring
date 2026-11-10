import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { api } from '../../lib/api'
import type { LoginResponse, User } from '../../lib/types'
import { useAppDispatch, useAppSelector } from '../../store'
import { signedIn, userLoaded } from '../../store/authSlice'

export interface Credentials {
  email: string
  password: string
}

function login(credentials: Credentials) {
  return api<LoginResponse>('/auth/login', { method: 'POST', body: credentials, token: null })
}

/** Logs in and stores the session. */
export function useLogin() {
  const dispatch = useAppDispatch()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: login,
    onSuccess: ({ token, user }) => {
      queryClient.clear() // nothing from a previous user may remain
      dispatch(signedIn({ token, user }))
    },
  })
}

/** Creates the account, then logs straight in with the same details. */
export function useSignup() {
  const dispatch = useAppDispatch()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (credentials: Credentials) => {
      await api<User>('/auth/signup', { method: 'POST', body: credentials, token: null })
      return login(credentials)
    },
    onSuccess: ({ token, user }) => {
      queryClient.clear()
      dispatch(signedIn({ token, user }))
    },
  })
}

/**
 * The signed-in user. After a page refresh only the token is known, so this
 * loads the user once from GET /auth/me and keeps it in the store.
 */
export function useSessionUser(): User | null {
  const dispatch = useAppDispatch()
  const { status, user } = useAppSelector((state) => state.auth)

  const me = useQuery({
    queryKey: ['me'],
    queryFn: ({ signal }) => api<User>('/auth/me', { signal }),
    enabled: status === 'signedIn' && user === null,
  })

  useEffect(() => {
    if (me.data) dispatch(userLoaded(me.data))
  }, [me.data, dispatch])

  return user
}
