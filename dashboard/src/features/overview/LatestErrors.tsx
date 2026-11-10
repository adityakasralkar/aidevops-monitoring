import { Link } from '@tanstack/react-router'
import { SeverityPill } from '../../components/ui/SeverityPill'
import { Skeleton } from '../../components/ui/Skeleton'
import { StatusDot } from '../../components/ui/StatusDot'
import { clockTime } from '../../lib/format'
import type { DiagnosisStatus } from '../../lib/types'
import type { Range } from './range'
import { useLatestErrors } from './queries'
import styles from './Overview.module.css'

const diagnosis: Record<DiagnosisStatus, { text: string; tone: 'quiet' | 'ok' | 'error' }> = {
  PENDING: { text: 'AI pending', tone: 'quiet' },
  COMPLETED: { text: 'Diagnosed', tone: 'ok' },
  FAILED: { text: 'Diagnosis failed', tone: 'error' },
}

/** The three newest serious errors, each linking to the Errors page. */
export function LatestErrors({ projectId, range }: { projectId: string; range: Range }) {
  const errors = useLatestErrors(projectId, range)

  return (
    <section className={`${styles.card} ${styles.listCard}`} aria-labelledby="latest-errors-title">
      <div className={`${styles.cardHead} ${styles.listHead}`}>
        <h2 id="latest-errors-title" className={styles.cardTitle}>
          Latest errors
        </h2>
        <Link to="/p/$projectId/errors" params={{ projectId }} className={styles.headLink}>
          View all
        </Link>
      </div>
      {errors.isPending && (
        <div className={styles.listBody}>
          <Skeleton width="80%" />
          <Skeleton width="60%" />
        </div>
      )}
      {errors.isSuccess && errors.data.length === 0 && (
        <p className={`${styles.listBody} ${styles.muted}`}>No ERROR or CRITICAL logs in this range.</p>
      )}
      {errors.isSuccess && errors.data.length > 0 && (
        <ul className={styles.errorList}>
          {errors.data.map((e) => {
            const d = diagnosis[e.diagnosis_status ?? 'PENDING']
            return (
              <li key={e.id}>
                <Link to="/p/$projectId/errors" params={{ projectId }} search={{ id: e.id, range }} className={styles.errorRow}>
                  <SeverityPill severity={e.severity} />
                  <span className={styles.errorText}>
                    <span className={styles.errorMessage}>{e.message}</span>
                    <span className={styles.errorMeta}>
                      {e.service} · {clockTime(e.timestamp)}
                    </span>
                  </span>
                  <span className={styles.diagnosis}>
                    <StatusDot tone={d.tone} />
                    {d.text}
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
