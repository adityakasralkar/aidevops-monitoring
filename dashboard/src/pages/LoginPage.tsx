import { useSearch } from '@tanstack/react-router'
import { useEffect } from 'react'
import { BrandPanel } from '../features/auth/BrandPanel'
import { LoginForm } from '../features/auth/LoginForm'
import styles from './LoginPage.module.css'

/** Log in / sign up, also shown (with a notice) when a session expires. */
export function LoginPage() {
  const { expired, redirect } = useSearch({ from: '/login' })

  useEffect(() => {
    document.title = 'Log in · AIDevOps'
  }, [])

  return (
    <div className={styles.page}>
      <BrandPanel expired={expired} />
      <main className={styles.formSide}>
        <LoginForm expired={expired} redirect={redirect} />
      </main>
    </div>
  )
}
