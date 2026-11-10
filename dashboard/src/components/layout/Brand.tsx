import styles from './Brand.module.css'

/** The wordmark: "ai" on a lime pill, then "devops". */
export function Brand() {
  return (
    <span className={styles.brand} aria-label="AIDevOps" role="img">
      <span className={styles.ai} aria-hidden="true">
        ai
      </span>
      <span aria-hidden="true">devops</span>
    </span>
  )
}
