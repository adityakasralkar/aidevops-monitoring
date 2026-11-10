import { Link, useRouterState } from '@tanstack/react-router'
import { useSessionUser } from '../../features/auth/queries'
import { useLogout } from '../../features/auth/useLogout'
import { LogOutIcon } from '../ui/icons/Icons'
import styles from './UserCard.module.css'

/** Bottom of the sidebar: who is signed in, a link to Account and Log out. */
export function UserCard() {
  const user = useSessionUser()
  const onAccountPage = useRouterState({ select: (s) => s.location.pathname === '/account' })
  const logOut = useLogout()

  const email = user?.email ?? 'Not signed in'
  const initial = user?.email.charAt(0).toUpperCase() ?? '?'

  return (
    <div className={onAccountPage ? `${styles.card} ${styles.active}` : styles.card}>
      <span className={styles.avatar} aria-hidden="true">
        {initial}
      </span>
      <span className={styles.text}>
        {user ? (
          <>
            <Link to="/account" className={styles.email} title="Account settings">
              {email}
            </Link>
            <button type="button" className={styles.logout} onClick={logOut}>
              <LogOutIcon size={12} />
              Log out
            </button>
          </>
        ) : (
          <>
            <span className={styles.email}>{email}</span>
            <Link to="/login" className={styles.logout}>
              Log in
            </Link>
          </>
        )}
      </span>
    </div>
  )
}
