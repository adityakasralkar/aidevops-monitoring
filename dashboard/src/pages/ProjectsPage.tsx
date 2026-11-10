import { useEffect } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { TopBar } from '../components/layout/TopBar'
import styles from './ProjectsPage.module.css'

/** Projects list. The list itself comes with the API client. */
export function ProjectsPage() {
  useEffect(() => {
    document.title = 'Projects · AIDevOps'
  }, [])

  return (
    <div className={styles.page}>
      <TopBar />
      <main className={styles.main}>
        <PageHeader title="Projects" />
      </main>
    </div>
  )
}
