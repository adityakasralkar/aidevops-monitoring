import type { ButtonHTMLAttributes } from 'react'
import styles from './Button.module.css'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Shows a busy state and blocks clicks while a request runs. */
  loading?: boolean
  fullWidth?: boolean
}

/** Primary (graphite) button. */
export function Button({ loading = false, fullWidth = false, className, children, ...rest }: ButtonProps) {
  const classes = [styles.button, fullWidth ? styles.full : '', className ?? ''].join(' ').trim()
  return (
    <button {...rest} className={classes} aria-busy={loading || undefined} disabled={rest.disabled || loading}>
      {children}
    </button>
  )
}
