import { Link } from '@tanstack/react-router'
import { useSessionUser } from '../../features/auth/queries'
import { useLogout } from '../../features/auth/useLogout'
import { Brand } from './Brand'
import styles from './TopBar.module.css'

/** Header for signed-in pages without a project open (the Projects list). */
export function TopBar() {
  const email = useSessionUser()?.email
  const logOut = useLogout()

  return (
    <header className={styles.bar}>
      <Brand />
      <div className={styles.right}>
        {email && (
          <Link to="/account" className={styles.email} title="Account settings">
            {email}
          </Link>
        )}
        <button type="button" className={styles.logout} onClick={logOut}>
          Log out
        </button>
      </div>
    </header>
  )
}
