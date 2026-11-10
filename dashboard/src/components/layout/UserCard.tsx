import { Link, useRouterState } from '@tanstack/react-router'
import { useSessionUser } from '../../features/auth/queries'
import { useLogout } from '../../features/auth/useLogout'
import { LogOutIcon } from '../ui/icons/Icons'
import styles from './UserCard.module.css'

/**
 * Bottom of the sidebar: who is signed in, a link to Account and Log out.
 * Only used inside signed-in pages; the email may still be loading after a refresh.
 */
export function UserCard() {
  const user = useSessionUser()
  const onAccountPage = useRouterState({ select: (s) => s.location.pathname === '/account' })
  const logOut = useLogout()

  return (
    <div className={onAccountPage ? `${styles.card} ${styles.active}` : styles.card}>
      <Link to="/account" className={styles.avatar} title={user?.email ?? 'Account'}>
        <span aria-hidden="true">{user ? user.email.charAt(0).toUpperCase() : ''}</span>
        <span className="sr-only">Account settings</span>
      </Link>
      <span className={styles.text}>
        {user ? (
          <Link to="/account" className={styles.email} title="Account settings">
            {user.email}
          </Link>
        ) : (
          <span className={styles.loading} aria-label="Loading account" />
        )}
        <button type="button" className={styles.logout} onClick={logOut} title="Log out">
          <LogOutIcon size={12} />
          <span className={styles.logoutText}>Log out</span>
        </button>
      </span>
    </div>
  )
}
