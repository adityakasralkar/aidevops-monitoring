import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import type { User } from '../lib/types'

/**
 * signedOut: no token.
 * signedIn:  we hold a token (the user may still be loading after a refresh).
 * expired:   the server rejected the token with 401; show the Session expired screen.
 */
export type AuthStatus = 'signedOut' | 'signedIn' | 'expired'

export interface AuthState {
  token: string | null
  user: User | null
  status: AuthStatus
}

/** sessionStorage key: survives a refresh, cleared when the tab closes. */
export const TOKEN_STORAGE_KEY = 'aidevops.session'

function readStoredToken(): string | null {
  try {
    return sessionStorage.getItem(TOKEN_STORAGE_KEY)
  } catch {
    return null // storage blocked (private mode, strict settings)
  }
}

function initialState(): AuthState {
  const token = readStoredToken()
  return { token, user: null, status: token ? 'signedIn' : 'signedOut' }
}

const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    signedIn(state, action: PayloadAction<{ token: string; user: User }>) {
      state.token = action.payload.token
      state.user = action.payload.user
      state.status = 'signedIn'
    },
    userLoaded(state, action: PayloadAction<User>) {
      state.user = action.payload
    },
    signedOut(state) {
      state.token = null
      state.user = null
      state.status = 'signedOut'
    },
    expired(state) {
      state.token = null
      state.user = null
      state.status = 'expired'
    },
  },
})

export const { signedIn, userLoaded, signedOut, expired } = authSlice.actions
export const authReducer = authSlice.reducer
