import { Link } from '@tanstack/react-router'
import { GlassCard } from '../../components/ui/GlassCard'
import { HistogramBars } from '../../components/ui/HistogramBars'
import { ChevronRightIcon } from '../../components/ui/icons/Icons'
import { LineChart } from '../../components/ui/LineChart'
import { Skeleton } from '../../components/ui/Skeleton'
import { apiErrorMessage } from '../../lib/errors'
import { axisTime, compactNumber } from '../../lib/format'
import type { MetricInfo, MetricSeries } from '../../lib/types'
import type { Range } from '../overview/range'
import { useMetricSeries } from './queries'
import type { MetricSearch } from './search'
import { boundLabel, formatValue, typeText } from './units'
import styles from './Metrics.module.css'

function xLabels(series: MetricSeries): string[] {
  const pts = series.points
  if (pts.length === 0) return []
  const spansDays = new Date(pts[pts.length - 1].t).getTime() - new Date(pts[0].t).getTime() > 24 * 3600 * 1000
  const count = Math.min(5, pts.length)
  return Array.from({ length: count }, (_, i) =>
    axisTime(pts[Math.round((i / Math.max(1, count - 1)) * (pts.length - 1))].t, spansDays),
  )
}

/** Headline numbers for the chosen metric, from the chart points. */
function stats(series: MetricSeries): { label: string; value: string }[] {
  const unit = series.unit
  if (series.type === 'histogram') {
    const total = series.points.reduce((sum, p) => sum + (p.count ?? 0), 0)
    const p95s = series.points.map((p) => p.p95).filter((v): v is number => v !== undefined)
    const last = [...series.points].reverse().find((p) => p.p95 !== undefined)
    return [
      { label: 'Recorded', value: compactNumber(total) },
      { label: 'Latest p95', value: formatValue(last?.p95, unit) },
      { label: 'Highest p95', value: formatValue(p95s.length ? Math.max(...p95s) : null, unit) },
    ]
  }
  const values = series.points.map((p) => p.value).filter((v): v is number => v !== undefined)
  const latest = [...series.points].reverse().find((p) => p.value !== undefined)?.value
  return [
    { label: 'Latest', value: formatValue(latest, unit) },
    { label: 'Lowest', value: formatValue(values.length ? Math.min(...values) : null, unit) },
    { label: 'Highest', value: formatValue(values.length ? Math.max(...values) : null, unit) },
  ]
}

interface MetricDetailProps {
  projectId: string
  metric: MetricInfo | undefined
  range: Range
  search: MetricSearch
}

/** Right side: the chosen metric's chart, numbers and (histograms) distribution. */
export function MetricDetail({ projectId, metric, range, search }: MetricDetailProps) {
  const series = useMetricSeries(projectId, metric, range)
  const { name: _n, service: _s, ...listSearch } = search
  const back = (
    <Link to="/p/$projectId/metrics" params={{ projectId }} search={listSearch} className={styles.back}>
      <ChevronRightIcon size={14} className={styles.backIcon} />
      All metrics
    </Link>
  )
  if (!metric) return null

  const data = series.data?.series
  const isHistogram = metric.type === 'histogram'
  const dist = data?.distribution
  const bars = dist
    ? {
        labels: [...dist.bounds.map((b) => `≤ ${boundLabel(b, metric.unit)}`), `> ${boundLabel(dist.bounds[dist.bounds.length - 1] ?? 0, metric.unit)}`],
        counts: dist.counts,
      }
    : null

  return (
    <div className={styles.detail}>
      {back}
      <GlassCard as="section">
        <div className={styles.detailHead}>
          <div className={styles.detailTitle}>
            <h2 className={styles.metricName}>{metric.name}</h2>
            <span className={styles.small}>
              {metric.service} · {typeText[metric.type]?.about ?? metric.type}
            </span>
          </div>
          {data && (
            <div className={styles.stats}>
              {stats(data).map((s) => (
                <div key={s.label} className={styles.stat}>
                  <span className={styles.statLabel}>{s.label}</span>
                  <span className={styles.statValue}>{s.value}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        {series.isPending && (
          <div className={styles.loading}>
            <Skeleton height={180} />
          </div>
        )}
        {series.isError && !data && <p className={styles.errorText}>{apiErrorMessage(series.error)}</p>}
        {data && (
          <>
            {isHistogram && (
              <div className={styles.legend}>
                <span>
                  <i className={styles.keyMain} /> p95
                </span>
                <span>
                  <i className={styles.keyMuted} /> p50
                </span>
              </div>
            )}
            <LineChart
              height={200}
              xLabels={xLabels(data)}
              summary={`${metric.name} for ${metric.service} over the selected range`}
              series={
                isHistogram
                  ? [
                      { label: 'p95', tone: 'main', values: data.points.map((p) => p.p95 ?? null) },
                      { label: 'p50', tone: 'muted', values: data.points.map((p) => p.p50 ?? null) },
                    ]
                  : [{ label: metric.name, tone: 'main', area: true, values: data.points.map((p) => p.value ?? null) }]
              }
            />
            {series.data?.truncated && (
              <p className={styles.small}>Showing the first part of a very busy range; pick a shorter range for every point.</p>
            )}
          </>
        )}
      </GlassCard>

      {isHistogram && bars && (
        <GlassCard as="section">
          <div className={styles.sectionHead}>
            <h3 className={styles.cardTitle}>Distribution</h3>
            <span className={styles.small}>How many values fell in each bucket</span>
          </div>
          <HistogramBars
            labels={bars.labels}
            counts={bars.counts}
            summary={`How ${metric.name} values are spread across buckets in the selected range`}
          />
        </GlassCard>
      )}
    </div>
  )
}
