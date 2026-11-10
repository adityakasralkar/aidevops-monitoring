import { redirect } from '@tanstack/react-router'
import { store } from '../../store'

/** Search params of the login page. */
export interface LoginSearch {
  /** Path to return to after logging in. */
  redirect?: string
  /** Show the "session expired" notice. */
  expired?: boolean
}

/**
 * Only same-site paths are allowed as a return address, so a crafted link
 * cannot send someone to another website after they log in.
 */
export function safeRedirect(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  if (!value.startsWith('/') || value.startsWith('//') || value.startsWith('/\\')) return undefined
  return value
}

export function parseLoginSearch(search: Record<string, unknown>): LoginSearch {
  return {
    redirect: safeRedirect(search.redirect),
    expired: search.expired === true || search.expired === 'true' ? true : undefined,
  }
}

/** Route guard: pages that need a signed-in user send everyone else to /login. */
export function requireSession({ location }: { location: { href: string } }) {
  const { status } = store.getState().auth
  if (status === 'signedIn') return
  throw redirect({
    to: '/login',
    search: { redirect: location.href, expired: status === 'expired' ? true : undefined },
  })
}

/** Route guard for /login: someone already signed in goes on to their projects. */
export function skipLoginWhenSignedIn({ search }: { search: LoginSearch }) {
  if (store.getState().auth.status !== 'signedIn') return
  if (search.redirect) throw redirect({ href: search.redirect })
  throw redirect({ to: '/projects' })
}
