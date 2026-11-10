import type { Severity } from '../../lib/types'
import styles from './SeverityPill.module.css'

/** CRITICAL / ERROR / WARN / INFO / DEBUG label. */
export function SeverityPill({ severity }: { severity: Severity }) {
  return <span className={`${styles.pill} ${styles[severity.toLowerCase()]}`}>{severity}</span>
}
