import { Link } from '@tanstack/react-router'
import { GlassCard } from '../components/ui/GlassCard'
import styles from './NotFoundPage.module.css'

export function NotFoundPage() {
  return (
    <main className={styles.page}>
      <GlassCard className={styles.card}>
        <h1 className={styles.title}>Page not found</h1>
        <p className={styles.text}>This address does not match any page.</p>
        <Link to="/projects">Go to your projects</Link>
      </GlassCard>
    </main>
  )
}
