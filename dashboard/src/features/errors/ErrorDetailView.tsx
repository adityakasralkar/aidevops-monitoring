import { Link } from '@tanstack/react-router'
import { GlassCard } from '../../components/ui/GlassCard'
import { ChevronRightIcon } from '../../components/ui/icons/Icons'
import { LineChart } from '../../components/ui/LineChart'
import { SeverityPill } from '../../components/ui/SeverityPill'
import { Skeleton } from '../../components/ui/Skeleton'
import { StatusDot } from '../../components/ui/StatusDot'
import { ApiError } from '../../lib/api'
import { apiErrorMessage } from '../../lib/errors'
import { compactNumber, dateTime, logTime, ms } from '../../lib/format'
import type { ErrorDetail, MetricSeries } from '../../lib/types'
import { diagnosisLabel } from './diagnosis'
import type { ErrorSearch } from './filters'
import { useErrorDetail } from './queries'
import styles from './Errors.module.css'

const MAX_CHARTS = 4

/** The number a metric chart draws: p95 for histograms, the value otherwise. */
function chartValues(series: MetricSeries): (number | null)[] {
  return series.points.map((p) => (series.type === 'histogram' ? (p.p95 ?? null) : (p.value ?? null)))
}

function lastValue(series: MetricSeries): string {
  const values = chartValues(series).filter((v): v is number => v !== null)
  if (values.length === 0) return 'no data'
  const v = values[values.length - 1]
  if (series.type === 'histogram') return `p95 ${ms(series.unit === 's' ? v * 1000 : v)}`
  return `${compactNumber(v)}${series.unit && !series.unit.startsWith('{') ? ` ${series.unit}` : ''}`
}

/** Metrics of the failing service first, then the others; at most four charts. */
function pickCharts(detail: ErrorDetail): MetricSeries[] {
  const withData = detail.context.metrics.filter((m) => chartValues(m).some((v) => v !== null))
  const same = withData.filter((m) => m.service === detail.service)
  const other = withData.filter((m) => m.service !== detail.service)
  return [...same, ...other].slice(0, MAX_CHARTS)
}

function AiCard({ detail }: { detail: ErrorDetail }) {
  const status = detail.diagnosis_status ?? 'PENDING'
  const d = diagnosisLabel[status]
  return (
    <section className={styles.aiCard} aria-labelledby="ai-title">
      <div className={styles.aiHead}>
        <div className={styles.aiTitleRow}>
          <span className={styles.aiBadge}>AI</span>
          <h3 id="ai-title" className={styles.cardTitle}>
            Diagnosis
          </h3>
        </div>
        <span className={styles.status}>
          <StatusDot tone={d.tone} />
          {d.text}
        </span>
      </div>
      <div className={styles.aiBody}>
        {status === 'COMPLETED' && (
          <>
            {detail.ai_root_cause && (
              <div className={styles.aiBlock}>
                <h4>Root cause</h4>
                <p>{detail.ai_root_cause}</p>
              </div>
            )}
            {detail.ai_suggested_fix && (
              <div className={styles.aiBlock}>
                <h4>Suggested fix</h4>
                <p className={styles.preLine}>{detail.ai_suggested_fix}</p>
              </div>
            )}
          </>
        )}
        {status === 'PENDING' && (
          <p>
            This error is stored with status <code>PENDING</code>, together with the logs and metrics from the 10
            minutes before it, ready for the AI diagnosis. Until then, use the context below to investigate.
          </p>
        )}
        {status === 'FAILED' && (
          <p>The AI's answer could not be read as a diagnosis, so nothing was saved. The error itself is stored safely.</p>
        )}
      </div>
      <p className={styles.aiFoot}>Suggestions only. AIDevOps never changes, restarts or patches your app.</p>
    </section>
  )
}

function Context({ detail }: { detail: ErrorDetail }) {
  const charts = pickCharts(detail)
  const logs = detail.context.related_logs.filter((l) => l.id !== detail.id)
  return (
    <>
      <GlassCard as="section">
        <div className={styles.sectionHead}>
          <h3 className={styles.cardTitle}>Before the error</h3>
          <span className={styles.small}>
            {logTime(detail.context.window_start)} – {logTime(detail.context.window_end)}
          </span>
        </div>
        {charts.length === 0 ? (
          <p className={styles.small}>No metrics were received in the 10 minutes before this error.</p>
        ) : (
          <div className={styles.charts}>
            {charts.map((m) => (
              <figure key={`${m.service}/${m.name}`} className={styles.chart}>
                <figcaption className={styles.chartTitle}>
                  <span className={styles.mono}>{m.name}</span>
                  <span className={styles.small}>
                    {m.service} · {lastValue(m)}
                  </span>
                </figcaption>
                <LineChart
                  height={80}
                  summary={`${m.name} for ${m.service} in the 10 minutes before the error`}
                  xLabels={['−10 min', 'error']}
                  series={[{ label: m.name, tone: 'main', values: chartValues(m) }]}
                />
              </figure>
            ))}
          </div>
        )}
      </GlassCard>

      <GlassCard as="section" padding="none">
        <div className={`${styles.sectionHead} ${styles.padded}`}>
          <h3 className={styles.cardTitle}>Logs from {detail.service} before it</h3>
          <span className={styles.small}>{logs.length}</span>
        </div>
        {logs.length === 0 ? (
          <p className={`${styles.small} ${styles.padded}`}>No other logs from this service in those 10 minutes.</p>
        ) : (
          <ul className={styles.related}>
            {logs.map((l) => (
              <li key={l.id}>
                <span className={styles.relatedTime}>{logTime(l.timestamp)}</span>
                <SeverityPill severity={l.severity} />
                <span className={styles.relatedMessage}>{l.message}</span>
              </li>
            ))}
          </ul>
        )}
      </GlassCard>
    </>
  )
}

interface ErrorDetailViewProps {
  projectId: string
  errorId: string | undefined
  search: ErrorSearch
}

/** Right side: everything about one error. */
export function ErrorDetailView({ projectId, errorId, search }: ErrorDetailViewProps) {
  const detail = useErrorDetail(projectId, errorId)
  const { id: _selected, ...listSearch } = search

  const back = (
    <Link to="/p/$projectId/errors" params={{ projectId }} search={listSearch} className={styles.back}>
      <ChevronRightIcon size={14} className={styles.backIcon} />
      Back to errors
    </Link>
  )

  if (!errorId) return null
  if (detail.isPending) {
    return (
      <div className={styles.detail}>
        {back}
        <GlassCard>
          <div className={styles.loading}>
            <Skeleton width="40%" />
            <Skeleton width="85%" height={16} />
          </div>
        </GlassCard>
      </div>
    )
  }
  if (detail.isError) {
    const gone = detail.error instanceof ApiError && detail.error.status === 404
    return (
      <div className={styles.detail}>
        {back}
        <GlassCard>
          <p role="alert" className={styles.errorText}>
            {gone ? "This error doesn't exist in this project." : apiErrorMessage(detail.error)}
          </p>
        </GlassCard>
      </div>
    )
  }

  const e = detail.data
  return (
    <div className={styles.detail}>
      {back}
      <GlassCard as="section">
        <div className={styles.headline}>
          <div className={styles.headMeta}>
            <SeverityPill severity={e.severity} />
            <span>{e.service}</span>
            <span aria-hidden="true">·</span>
            <span>{dateTime(e.timestamp)}</span>
            <span aria-hidden="true">·</span>
            <span className={styles.mono}>{e.id}</span>
          </div>
          <h2 className={styles.headMessage}>{e.message}</h2>
        </div>
      </GlassCard>

      <AiCard detail={e} />
      <Context detail={e} />

      {e.stack_trace && (
        <GlassCard as="section">
          <h3 className={styles.cardTitle}>Stack trace</h3>
          <pre className={styles.stack} tabIndex={0}>
            {e.stack_trace}
          </pre>
        </GlassCard>
      )}
    </div>
  )
}
