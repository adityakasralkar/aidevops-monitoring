import { useEffect } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { Button } from '../components/ui/Button'
import { GlassCard } from '../components/ui/GlassCard'
import { Skeleton } from '../components/ui/Skeleton'
import styles from '../features/account/Account.module.css'
import { ChangePasswordForm } from '../features/account/ChangePasswordForm'
import { useSessionUser } from '../features/auth/queries'
import { useLogout } from '../features/auth/useLogout'
import { dateOnly } from '../lib/format'

/** Your login details: profile, password, log out. Shared by all your projects. */
export function AccountPage() {
  const user = useSessionUser()
  const logOut = useLogout()

  useEffect(() => {
    document.title = 'Account · AIDevOps'
  }, [])

  return (
    <>
      <PageHeader title="Account" subtitle="Your login details. These apply to all your projects." />
      <div className={styles.column}>
        <GlassCard as="section">
          <div className={styles.section}>
            <h2 className={styles.title}>Profile</h2>
            {user ? (
              <dl className={styles.facts}>
                <div>
                  <dt>Email</dt>
                  <dd>{user.email}</dd>
                </div>
                <div>
                  <dt>Member since</dt>
                  <dd>{dateOnly(user.created_at)}</dd>
                </div>
              </dl>
            ) : (
              <div className={styles.loading} aria-label="Loading profile">
                <Skeleton width="50%" />
              </div>
            )}
          </div>
        </GlassCard>

        <ChangePasswordForm />

        <GlassCard as="section">
          <div className={styles.row}>
            <div className={styles.heading}>
              <h2 className={styles.title}>Log out</h2>
              <p className={styles.text}>Ends your session on this device.</p>
            </div>
            <Button variant="secondary" onClick={logOut}>
              Log out
            </Button>
          </div>
        </GlassCard>
      </div>
    </>
  )
}
