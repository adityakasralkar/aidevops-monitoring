import { Link } from '@tanstack/react-router'
import { useNow } from '../../hooks/useNow'
import { compactNumber, percent, timeAgo } from '../../lib/format'
import type { OverviewService } from '../../lib/types'
import { languageName } from '../connect/queries'
import styles from './Overview.module.css'

/** Every program that sent data in the range, with its traffic and errors. */
export function ServicesTable({ projectId, services }: { projectId: string; services: OverviewService[] }) {
  const now = useNow()
  return (
    <section className={`${styles.card} ${styles.listCard} ${styles.servicesCard}`} aria-labelledby="services-title">
      <div className={`${styles.cardHead} ${styles.listHead}`}>
        <h2 id="services-title" className={styles.cardTitle}>
          Services
        </h2>
        <Link to="/p/$projectId/connect" params={{ projectId }} className={styles.headLink}>
          Connect another app
        </Link>
      </div>
      {services.length === 0 ? (
        <p className={`${styles.listBody} ${styles.muted}`}>No service sent data in this range.</p>
      ) : (
        <div className={styles.tableScroll}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th scope="col">Service</th>
                <th scope="col">Language</th>
                <th scope="col" className={styles.num}>
                  Req/min
                </th>
                <th scope="col" className={styles.num}>
                  5xx
                </th>
                <th scope="col" className={styles.num}>
                  Errors
                </th>
                <th scope="col" className={styles.num}>
                  Last seen
                </th>
              </tr>
            </thead>
            <tbody>
              {services.map((s) => (
                <tr key={s.name}>
                  <th scope="row" className={styles.serviceName}>
                    {s.name}
                  </th>
                  <td className={styles.muted}>{languageName(s.language)}</td>
                  <td className={styles.num}>{s.requests_per_min > 0 ? compactNumber(s.requests_per_min) : '—'}</td>
                  <td className={`${styles.num} ${(s.error_rate ?? 0) >= 0.01 ? styles.hot : ''}`}>{percent(s.error_rate)}</td>
                  <td className={`${styles.num} ${s.error_logs > 0 ? styles.hot : ''}`}>{compactNumber(s.error_logs)}</td>
                  <td className={`${styles.num} ${styles.muted}`}>{timeAgo(s.last_seen, now)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className={styles.footnote}>Services appear here by themselves when their first data arrives.</p>
    </section>
  )
}
