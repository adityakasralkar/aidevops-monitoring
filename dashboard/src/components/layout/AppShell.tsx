import { Outlet } from '@tanstack/react-router'
import { Sidebar } from './Sidebar'
import styles from './AppShell.module.css'

/** Layout for signed-in pages: sticky sidebar on the left, page on the right. */
export function AppShell() {
  return (
    <div className={styles.shell}>
      <Sidebar />
      <main className={styles.main}>
        <Outlet />
      </main>
    </div>
  )
}
