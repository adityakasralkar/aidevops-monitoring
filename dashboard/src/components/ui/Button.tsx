import type { ButtonHTMLAttributes } from 'react'
import styles from './Button.module.css'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** primary: graphite (one per screen). secondary: white with a border. */
  variant?: 'primary' | 'secondary'
  /** Shows a busy state and blocks clicks while a request runs. */
  loading?: boolean
  fullWidth?: boolean
}

export function Button({
  variant = 'primary',
  loading = false,
  fullWidth = false,
  className,
  children,
  ...rest
}: ButtonProps) {
  const classes = [styles.button, styles[variant], fullWidth ? styles.full : '', className ?? ''].join(' ').trim()
  return (
    <button {...rest} className={classes} aria-busy={loading || undefined} disabled={rest.disabled || loading}>
      {children}
    </button>
  )
}
