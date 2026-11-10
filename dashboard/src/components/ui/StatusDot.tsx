import styles from './StatusDot.module.css'

export type StatusTone = 'ok' | 'idle' | 'quiet' | 'error'

/** Small coloured dot in front of a status text. Always pair it with words. */
export function StatusDot({ tone }: { tone: StatusTone }) {
  return <span className={`${styles.dot} ${styles[tone]}`} aria-hidden="true" />
}
