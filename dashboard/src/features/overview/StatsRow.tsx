import { StatCard } from '../../components/ui/StatCard'
import { axisTime, compactNumber, dateTime, ms, percent } from '../../lib/format'
import type { Overview } from '../../lib/types'
import styles from './Overview.module.css'

/** The four headline numbers. Only real values; "—" when there is no data. */
export function StatsRow({ data }: { data: Overview }) {
  const http = data.has_http_metrics && data.requests > 0
  const perStepToPerMin = 60 / Math.max(1, data.step_seconds)
  const peak = data.series.reduce<{ value: number; t: string } | null>(
    (best, step) => (step.requests > 0 && (!best || step.requests > best.value) ? { value: step.requests, t: step.t } : best),
    null,
  )
  const failed = data.series.reduce((sum, step) => sum + step.errors, 0)
  const highErrorRate = (data.error_rate ?? 0) >= 0.01
  const spansDays = new Date(data.until).getTime() - new Date(data.since).getTime() > 24 * 3600 * 1000
  const peakAt = (t: string) => (spansDays ? dateTime(t) : axisTime(t, false))

  return (
    <div className={styles.stats}>
      <StatCard
        label="Requests"
        value={http ? compactNumber(data.requests_per_min) : '—'}
        unit={http ? '/min' : undefined}
        tone={http ? 'default' : 'empty'}
        note={
          http
            ? peak
              ? `${compactNumber(data.requests)} total · peak ${compactNumber(peak.value * perStepToPerMin)}/min at ${peakAt(peak.t)}`
              : `${compactNumber(data.requests)} total`
            : 'No request data yet'
        }
      />
      <StatCard
        label="Error rate"
        value={http ? percent(data.error_rate) : '—'}
        tone={http ? (highErrorRate ? 'error' : 'default') : 'empty'}
        note={http ? `${compactNumber(failed)} of ${compactNumber(data.requests)} got a 5xx` : 'No request data yet'}
      />
      <StatCard
        label="p95 latency"
        value={http ? ms(data.p95_ms) : '—'}
        tone={http ? 'default' : 'empty'}
        note={http ? `p50 ${ms(data.p50_ms)}` : 'No request data yet'}
      />
      <StatCard
        label="Errors to review"
        value={compactNumber(data.errors)}
        note={
          data.errors > 0 ? (
            <>
              <span className={styles.aiBadge}>AI</span>
              {compactNumber(data.errors_pending)} waiting for diagnosis
            </>
          ) : (
            'No errors in this range'
          )
        }
      />
    </div>
  )
}
