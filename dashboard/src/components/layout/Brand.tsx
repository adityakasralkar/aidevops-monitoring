import styles from './Brand.module.css'

interface BrandProps {
  /** `dark` for use on the dark login panel. */
  tone?: 'light' | 'dark'
  /** On tablets (icon rail) show only the "ai" pill. */
  collapsible?: boolean
}

/** The wordmark: "ai" on a lime pill, then "devops". */
export function Brand({ tone = 'light', collapsible = false }: BrandProps) {
  const classes = [styles.brand, tone === 'dark' ? styles.dark : '', collapsible ? styles.collapsible : '']
    .join(' ')
    .trim()
  return (
    <span className={classes} aria-label="AIDevOps" role="img">
      <span className={styles.ai} aria-hidden="true">
        ai
      </span>
      <span className={styles.rest} aria-hidden="true">
        devops
      </span>
    </span>
  )
}
