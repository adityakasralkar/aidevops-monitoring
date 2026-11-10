import { configureStore } from '@reduxjs/toolkit'
import { useDispatch, useSelector } from 'react-redux'
import { authReducer, TOKEN_STORAGE_KEY } from './authSlice'

// App state only. Data from the server lives in TanStack Query, not here.
export const store = configureStore({
  reducer: {
    auth: authReducer,
  },
})

export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch

// Typed versions of the react-redux hooks; components use these.
export const useAppDispatch = useDispatch.withTypes<AppDispatch>()
export const useAppSelector = useSelector.withTypes<RootState>()

// Keep the token in sessionStorage in step with the store. Reducers stay
// pure; this listener does the side effect.
let savedToken = store.getState().auth.token
store.subscribe(() => {
  const { token } = store.getState().auth
  if (token === savedToken) return
  savedToken = token
  try {
    if (token) sessionStorage.setItem(TOKEN_STORAGE_KEY, token)
    else sessionStorage.removeItem(TOKEN_STORAGE_KEY)
  } catch {
    // Storage blocked: the session simply won't survive a refresh.
  }
})
