import styles from './Brand.module.css'

interface BrandProps {
  /** `dark` for use on the dark login panel. */
  tone?: 'light' | 'dark'
}

/** The wordmark: "ai" on a lime pill, then "devops". */
export function Brand({ tone = 'light' }: BrandProps) {
  return (
    <span className={tone === 'dark' ? `${styles.brand} ${styles.dark}` : styles.brand} aria-label="AIDevOps" role="img">
      <span className={styles.ai} aria-hidden="true">
        ai
      </span>
      <span aria-hidden="true">devops</span>
    </span>
  )
}
