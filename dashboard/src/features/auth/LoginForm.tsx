import { useNavigate } from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'
import { Button } from '../../components/ui/Button'
import { GlassCard } from '../../components/ui/GlassCard'
import { ClockIcon } from '../../components/ui/icons/Icons'
import { TextField } from '../../components/ui/TextField'
import { useAppSelector } from '../../store'
import { authErrorMessage } from './messages'
import { useLogin, useSignup, type Credentials } from './queries'
import styles from './LoginForm.module.css'

type Mode = 'login' | 'signup'
type FieldErrors = Partial<Record<keyof Credentials, string>>

const MIN_PASSWORD = 8
const MAX_PASSWORD = 72

/** Same length rules as the server, checked before sending. */
function validate({ email, password }: Credentials, mode: Mode): FieldErrors {
  const errors: FieldErrors = {}
  if (!email.trim()) errors.email = 'Enter your email.'
  else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) errors.email = 'Enter a valid email address.'

  if (!password) errors.password = 'Enter your password.'
  else if (mode === 'signup' && password.length < MIN_PASSWORD) errors.password = `Use at least ${MIN_PASSWORD} characters.`
  else if (mode === 'signup' && password.length > MAX_PASSWORD) errors.password = `Use ${MAX_PASSWORD} characters or fewer.`
  return errors
}

interface LoginFormProps {
  /** The session ran out: show the notice and prefill the last email. */
  expired?: boolean
  /** Where to go after logging in. */
  redirect?: string
}

export function LoginForm({ expired = false, redirect }: LoginFormProps) {
  const navigate = useNavigate()
  const lastEmail = useAppSelector((state) => state.auth.lastEmail)
  const login = useLogin()
  const signup = useSignup()

  const [mode, setMode] = useState<Mode>('login')
  const [showExpired, setShowExpired] = useState(expired)
  const [email, setEmail] = useState(expired ? (lastEmail ?? '') : '')
  const [password, setPassword] = useState('')
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})

  const mutation = mode === 'login' ? login : signup
  const isLogin = mode === 'login'

  const switchMode = (next: Mode) => {
    setMode(next)
    setFieldErrors({})
    login.reset()
    signup.reset()
  }

  const useOtherAccount = () => {
    setShowExpired(false)
    setEmail('')
    setPassword('')
    switchMode('login')
  }

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    const credentials = { email: email.trim(), password }
    const errors = validate(credentials, mode)
    setFieldErrors(errors)
    if (Object.keys(errors).length > 0) return

    mutation.mutate(credentials, {
      onSuccess: () => {
        if (redirect) void navigate({ href: redirect, replace: true })
        else void navigate({ to: '/projects', replace: true })
      },
    })
  }

  const title = showExpired ? 'Log in again' : isLogin ? 'Welcome back' : 'Create your account'
  const subtitle = showExpired
    ? "You'll go back to where you were."
    : isLogin
      ? 'Log in to see your projects.'
      : "You'll be signed in right after."

  return (
    <GlassCard className={styles.card}>
      {showExpired && (
        <div role="status" className={styles.notice}>
          <ClockIcon className={styles.noticeIcon} />
          <span>
            <strong>Your session expired.</strong> For safety, logins last 24 hours. Log in again to continue where you
            were.
          </span>
        </div>
      )}

      <div className={styles.heading}>
        <h2 className={styles.title}>{title}</h2>
        <p className={styles.subtitle}>{subtitle}</p>
      </div>

      {!showExpired && (
        <div role="tablist" aria-label="Account" className={styles.tabs}>
          <button
            type="button"
            role="tab"
            aria-selected={isLogin}
            className={styles.tab}
            onClick={() => switchMode('login')}
          >
            Log in
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={!isLogin}
            className={styles.tab}
            onClick={() => switchMode('signup')}
          >
            Sign up
          </button>
        </div>
      )}

      <form className={styles.form} onSubmit={onSubmit} noValidate>
        <div className={styles.fields}>
          <TextField
            label="Email"
            type="email"
            name="email"
            autoComplete="email"
            placeholder="you@company.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            error={fieldErrors.email}
          />
          <TextField
            label="Password"
            type="password"
            name="password"
            autoComplete={isLogin ? 'current-password' : 'new-password'}
            placeholder="••••••••"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            hint={isLogin ? undefined : `At least ${MIN_PASSWORD} characters.`}
            error={fieldErrors.password}
          />
        </div>

        {mutation.isError && (
          <p role="alert" className={styles.formError}>
            {authErrorMessage(mutation.error)}
          </p>
        )}

        <Button type="submit" fullWidth loading={mutation.isPending}>
          {mutation.isPending ? (isLogin ? 'Logging in…' : 'Creating account…') : isLogin ? 'Log in' : 'Create account'}
        </Button>

        {showExpired && (
          <button type="button" className={styles.linkButton} onClick={useOtherAccount}>
            Use a different account
          </button>
        )}
      </form>
    </GlassCard>
  )
}
