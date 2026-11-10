import { Link } from '@tanstack/react-router'
import { useLogout } from '../../features/auth/useLogout'
import { useAppSelector } from '../../store'
import { Brand } from './Brand'
import styles from './TopBar.module.css'

/** Header for pages without a project open (the Projects list). */
export function TopBar() {
  const email = useAppSelector((state) => state.auth.user?.email)
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
