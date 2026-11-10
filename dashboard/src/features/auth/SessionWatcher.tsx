import { useNavigate, useRouterState } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useAppSelector } from '../../store'

/**
 * When the server rejects the token (401) while you are on a page, go to
 * "Log in again" and come back to the same page afterwards.
 */
export function SessionWatcher() {
  const status = useAppSelector((state) => state.auth.status)
  const location = useRouterState({ select: (s) => s.location })
  const navigate = useNavigate()

  useEffect(() => {
    if (status !== 'expired' || location.pathname === '/login') return
    void navigate({ to: '/login', search: { expired: true, redirect: location.href }, replace: true })
  }, [status, location.pathname, location.href, navigate])

  return null
}
