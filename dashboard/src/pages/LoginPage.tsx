import { useEffect } from 'react'
import { Brand } from '../components/layout/Brand'
import { GlassCard } from '../components/ui/GlassCard'
import styles from './LoginPage.module.css'

/** Log in / Sign up. The forms come in the next step. */
export function LoginPage() {
  useEffect(() => {
    document.title = 'Log in · AIDevOps'
  }, [])

  return (
    <main className={styles.page}>
      <GlassCard className={styles.card}>
        <Brand />
      </GlassCard>
    </main>
  )
}
