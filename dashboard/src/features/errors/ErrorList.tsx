import { Link } from '@tanstack/react-router'
import { SeverityPill } from '../../components/ui/SeverityPill'
import { StatusDot } from '../../components/ui/StatusDot'
import { useNow } from '../../hooks/useNow'
import { logTime } from '../../lib/format'
import type { LogEntry } from '../../lib/types'
import { diagnosisLabel } from './diagnosis'
import type { ErrorSearch } from './filters'
import styles from './Errors.module.css'

interface ErrorListProps {
  projectId: string
  errors: LogEntry[]
  selectedId: string | undefined
  search: ErrorSearch
}

/** The error list; each item selects that error (kept in the URL). */
export function ErrorList({ projectId, errors, selectedId, search }: ErrorListProps) {
  const now = useNow()
  return (
    <ul className={styles.list} aria-label="Errors">
      {errors.map((e) => {
        const d = diagnosisLabel[e.diagnosis_status ?? 'PENDING']
        const selected = e.id === selectedId
        return (
          <li key={e.id}>
            <Link
              to="/p/$projectId/errors"
              params={{ projectId }}
              search={{ ...search, id: e.id }}
              replace
              className={selected ? `${styles.item} ${styles.selected}` : styles.item}
              aria-current={selected ? 'true' : undefined}
            >
              <span className={styles.itemTop}>
                <SeverityPill severity={e.severity} />
                <span className={styles.itemTime}>{logTime(e.timestamp, now)}</span>
              </span>
              <span className={styles.itemMessage}>{e.message}</span>
              <span className={styles.itemBottom}>
                <span>{e.service}</span>
                <span className={styles.itemDiagnosis}>
                  <StatusDot tone={d.tone} />
                  {d.text}
                </span>
              </span>
            </Link>
          </li>
        )
      })}
    </ul>
  )
}
