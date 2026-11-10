import { LineChart } from '../../components/ui/LineChart'
import { axisTime } from '../../lib/format'
import type { Overview } from '../../lib/types'
import styles from './Overview.module.css'

/** Five evenly spaced time labels under a chart. */
function labels(data: Overview): string[] {
  const steps = data.series
  if (steps.length === 0) return []
  const spansDays = new Date(data.until).getTime() - new Date(data.since).getTime() > 24 * 3600 * 1000
  const count = Math.min(5, steps.length)
  return Array.from({ length: count }, (_, i) =>
    axisTime(steps[Math.round((i / Math.max(1, count - 1)) * (steps.length - 1))].t, spansDays),
  )
}

/** Traffic + 5xx errors, and p95 / p50 latency over the range. */
export function Charts({ data }: { data: Overview }) {
  const x = labels(data)
  const perMin = 60 / Math.max(1, data.step_seconds)
  return (
    <div className={styles.charts}>
      <section className={styles.card} aria-labelledby="traffic-title">
        <div className={styles.cardHead}>
          <h2 id="traffic-title" className={styles.cardTitle}>
            Traffic and errors
          </h2>
          <div className={styles.legend}>
            <span>
              <i className={styles.keyMain} /> Requests
            </span>
            <span>
              <i className={styles.keyError} /> Errors
            </span>
          </div>
        </div>
        <LineChart
          summary="Requests per minute and 5xx errors over the selected range"
          xLabels={x}
          series={[
            { label: 'Requests', tone: 'main', area: true, values: data.series.map((s) => s.requests * perMin) },
            { label: 'Errors', tone: 'error', values: data.series.map((s) => s.errors * perMin) },
          ]}
        />
      </section>

      <section className={styles.card} aria-labelledby="latency-title">
        <div className={styles.cardHead}>
          <h2 id="latency-title" className={styles.cardTitle}>
            Latency
          </h2>
          <div className={styles.legend}>
            <span>
              <i className={styles.keyMain} /> p95
            </span>
            <span>
              <i className={styles.keyMuted} /> p50
            </span>
          </div>
        </div>
        <LineChart
          summary="p95 and p50 request latency in milliseconds over the selected range"
          xLabels={x}
          series={[
            { label: 'p95', tone: 'main', values: data.series.map((s) => s.p95_ms) },
            { label: 'p50', tone: 'muted', values: data.series.map((s) => s.p50_ms) },
          ]}
        />
      </section>
    </div>
  )
}
