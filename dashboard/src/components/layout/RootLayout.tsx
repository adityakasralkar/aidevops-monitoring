import { Outlet } from '@tanstack/react-router'
import { SessionWatcher } from '../../features/auth/SessionWatcher'

/** Wraps every page: watches for an expired session, then renders the page. */
export function RootLayout() {
  return (
    <>
      <SessionWatcher />
      <Outlet />
    </>
  )
}
