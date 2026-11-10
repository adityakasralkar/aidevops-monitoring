import { Link } from '@tanstack/react-router'
import { MetricsIcon } from '../../components/ui/icons/Icons'
import styles from './Overview.module.css'

/** Shown instead of the charts until the app sends HTTP request metrics. */
export function NoRequestData({ projectId, hasAnyData }: { projectId: string; hasAnyData: boolean }) {
  return (
    <section className={`${styles.card} ${styles.empty}`} aria-labelledby="no-requests-title">
      <span className={styles.emptyIcon} aria-hidden="true">
        <MetricsIcon size={20} />
      </span>
      <h2 id="no-requests-title" className={styles.emptyTitle}>
        No request data yet
      </h2>
      <p className={styles.emptyText}>
        Connect your app with OpenTelemetry to see traffic, error rate and latency here. Charts fill in by themselves
        when the first HTTP metrics arrive.
      </p>
      <div className={styles.emptyActions}>
        <Link to="/p/$projectId/connect" params={{ projectId }} className={styles.primaryLink}>
          Connect app
        </Link>
        <Link to="/p/$projectId/logs" params={{ projectId }} className={styles.secondaryLink}>
          View logs
        </Link>
      </div>
      {hasAnyData && (
        <p className={styles.footnote}>Sending only logs? Logs and Errors still work — this page needs HTTP metrics.</p>
      )}
    </section>
  )
}
