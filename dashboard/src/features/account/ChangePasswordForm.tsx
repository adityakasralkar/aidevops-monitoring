import { useState, type ChangeEvent, type FormEvent } from 'react'
import { Button } from '../../components/ui/Button'
import { GlassCard } from '../../components/ui/GlassCard'
import { TextField } from '../../components/ui/TextField'
import { apiErrorMessage } from '../../lib/errors'
import { useChangePassword } from './queries'
import styles from './Account.module.css'

const MIN = 8
const MAX = 72

type Field = 'current' | 'next' | 'again'

/** Current password, new password twice; checked here with the server's rules first. */
export function ChangePasswordForm() {
  const change = useChangePassword()
  const [values, setValues] = useState<Record<Field, string>>({ current: '', next: '', again: '' })
  const [errors, setErrors] = useState<Partial<Record<Field, string>>>({})

  const set = (field: Field) => (event: ChangeEvent<HTMLInputElement>) => {
    setValues((v) => ({ ...v, [field]: event.target.value }))
    setErrors((e) => ({ ...e, [field]: undefined }))
    if (change.isSuccess || change.isError) change.reset()
  }

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    const found: Partial<Record<Field, string>> = {}
    if (!values.current) found.current = 'Enter your current password.'
    if (values.next.length < MIN || values.next.length > MAX) found.next = `Use ${MIN} to ${MAX} characters.`
    else if (values.next === values.current) found.next = 'Choose a password different from the current one.'
    if (!found.next && values.again !== values.next) found.again = "The new passwords don't match."
    setErrors(found)
    if (Object.keys(found).length > 0) return

    change.mutate(
      { current_password: values.current, new_password: values.next },
      { onSuccess: () => setValues({ current: '', next: '', again: '' }) },
    )
  }

  return (
    <GlassCard as="section">
      <form className={styles.section} onSubmit={onSubmit} noValidate>
        <div className={styles.heading}>
          <h2 className={styles.title}>Change password</h2>
          <p className={styles.text}>Your other devices will be logged out. This one stays logged in.</p>
        </div>
        <div className={styles.fields}>
          <TextField
            label="Current password"
            type="password"
            autoComplete="current-password"
            value={values.current}
            onChange={set('current')}
            error={errors.current}
          />
          <TextField
            label="New password"
            type="password"
            autoComplete="new-password"
            value={values.next}
            onChange={set('next')}
            hint={`${MIN} to ${MAX} characters.`}
            error={errors.next}
          />
          <TextField
            label="Confirm new password"
            type="password"
            autoComplete="new-password"
            value={values.again}
            onChange={set('again')}
            error={errors.again}
          />
        </div>
        {change.isError && (
          <p role="alert" className={styles.formError}>
            {apiErrorMessage(change.error, { wrong_password: 'Your current password is wrong.' })}
          </p>
        )}
        {change.isSuccess && (
          <p role="status" className={styles.success}>
            Password updated. Your other devices were logged out.
          </p>
        )}
        <Button type="submit" className={styles.submit} loading={change.isPending}>
          {change.isPending ? 'Updating…' : 'Update password'}
        </Button>
      </form>
    </GlassCard>
  )
}
