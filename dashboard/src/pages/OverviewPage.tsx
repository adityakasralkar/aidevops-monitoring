import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useEffect } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { Button } from '../components/ui/Button'
import { GlassCard } from '../components/ui/GlassCard'
import { SegmentedControl } from '../components/ui/SegmentedControl'
import { Skeleton } from '../components/ui/Skeleton'
import { StatusDot } from '../components/ui/StatusDot'
import { useNow } from '../hooks/useNow'
import { Charts } from '../features/overview/Charts'
import { LatestErrors } from '../features/overview/LatestErrors'
import { NoRequestData } from '../features/overview/NoRequestData'
import { useOverview } from '../features/overview/queries'
import { DEFAULT_RANGE, RANGES } from '../features/overview/range'
import { ServicesTable } from '../features/overview/ServicesTable'
import { StatsRow } from '../features/overview/StatsRow'
import styles from '../features/overview/Overview.module.css'
import { apiErrorMessage } from '../lib/errors'
import { duration, secondsBetween, timeAgo } from '../lib/format'

/** A project's health at a glance: traffic, errors, latency, services. */
export function OverviewPage() {
  const { projectId } = useParams({ from: '/shell/p/$projectId/' })
  const range = useSearch({ from: '/shell/p/$projectId/' }).range ?? DEFAULT_RANGE
  const navigate = useNavigate({ from: '/p/$projectId/' })
  const overview = useOverview(projectId, range)
  const now = useNow(15_000)

  useEffect(() => {
    document.title = 'Overview · AIDevOps'
  }, [])

  const data = overview.data
  const last = data?.last_data_at ?? null
  const quiet = last ? secondsBetween(new Date(last), now) : null

  const status = !data ? null : last === null ? (
    <span className={styles.statusLine}>
      <StatusDot tone="idle" />
      No data received yet
    </span>
  ) : quiet !== null && quiet <= 300 ? (
    <span className={styles.statusLine}>
      <StatusDot tone="ok" />
      Receiving data · last data {timeAgo(last, now).toLowerCase()}
    </span>
  ) : (
    <span className={styles.statusLine}>
      <StatusDot tone="quiet" />
      No data for {duration(quiet ?? 0)}
    </span>
  )

  return (
    <>
      <PageHeader
        title="Overview"
        subtitle={status}
        actions={
          <SegmentedControl
            label="Time range"
            options={RANGES}
            value={range}
            onChange={(next) => void navigate({ search: { range: next }, replace: true })}
          />
        }
      />

      {overview.isPending && (
        <GlassCard padding="none">
          <div className={styles.loadingCard} aria-label="Loading overview">
            <Skeleton width="35%" height={16} />
            <Skeleton width="80%" />
            <Skeleton width="60%" />
          </div>
        </GlassCard>
      )}

      {overview.isError && !data && (
        <GlassCard>
          <div role="alert" className={styles.errorBox}>
            <p>{apiErrorMessage(overview.error)}</p>
            <Button variant="secondary" onClick={() => overview.refetch()}>
              Try again
            </Button>
          </div>
        </GlassCard>
      )}

      {data && (
        <>
          <StatsRow data={data} />
          {data.has_http_metrics ? <Charts data={data} /> : <NoRequestData projectId={projectId} hasAnyData={last !== null} />}
          {last !== null && (
            <div className={styles.lists}>
              <LatestErrors projectId={projectId} range={range} />
              <ServicesTable projectId={projectId} services={data.services} />
            </div>
          )}
        </>
      )}
    </>
  )
}
