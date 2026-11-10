import { Link } from '@tanstack/react-router'
import { useSessionUser } from '../../features/auth/queries'
import { useLogout } from '../../features/auth/useLogout'
import { Brand } from './Brand'
import styles from './TopBar.module.css'

/** Header for pages without a project open (the Projects list). */
export function TopBar() {
  const email = useSessionUser()?.email
  const logOut = useLogout()

  return (
    <header className={styles.bar}>
      <Brand />
      <div className={styles.right}>
        {email ? (
          <>
            <span className={styles.email}>{email}</span>
            <button type="button" className={styles.logout} onClick={logOut}>
              Log out
            </button>
          </>
        ) : (
          <Link to="/login" className={styles.logout}>
            Log in
          </Link>
        )}
      </div>
    </header>
  )
}
