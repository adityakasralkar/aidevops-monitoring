import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { Button } from '../components/ui/Button'
import { GlassCard } from '../components/ui/GlassCard'
import { Skeleton } from '../components/ui/Skeleton'
import { useServices } from '../features/connect/queries'
import { rangeLabels, type LogFilters } from '../features/logs/filters'
import { LogFiltersBar } from '../features/logs/LogFiltersBar'
import { LogList } from '../features/logs/LogList'
import { useLogs } from '../features/logs/queries'
import styles from '../features/logs/Logs.module.css'
import { DEFAULT_RANGE } from '../features/overview/range'
import { apiErrorMessage } from '../lib/errors'

/** Every log line of a project, newest first, with search and filters. */
export function LogsPage() {
  const { projectId } = useParams({ from: '/shell/p/$projectId/logs' })
  const search = useSearch({ from: '/shell/p/$projectId/logs' })
  const navigate = useNavigate({ from: '/p/$projectId/logs' })
  const [follow, setFollow] = useState(true)

  const filters = { ...search, range: search.range ?? DEFAULT_RANGE }
  const logs = useLogs(projectId, filters, follow)
  const services = useServices(projectId)

  useEffect(() => {
    document.title = 'Logs · AIDevOps'
  }, [])

  const setFilters = (next: LogFilters) => void navigate({ search: next, replace: true })
  const items = logs.data?.pages.flatMap((p) => p.items) ?? []
  const filtered = Boolean(search.level || search.service || search.q)
  const range = rangeLabels[filters.range].toLowerCase()

  return (
    <>
      <PageHeader
        title="Logs"
        subtitle={logs.isSuccess ? `${items.length}${logs.hasNextPage ? '+' : ''} shown · ${range}` : range}
        actions={
          <label className={styles.follow}>
            <input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} />
            Follow new logs
          </label>
        }
      />

      <LogFiltersBar
        filters={filters}
        services={(services.data ?? []).map((s) => s.name)}
        onChange={setFilters}
      />

      {logs.isPending && (
        <GlassCard padding="none">
          <div className={styles.loading} aria-label="Loading logs">
            <Skeleton width="90%" />
            <Skeleton width="75%" />
            <Skeleton width="85%" />
          </div>
        </GlassCard>
      )}

      {logs.isError && !logs.data && (
        <GlassCard>
          <div role="alert" className={styles.errorBox}>
            <p>{apiErrorMessage(logs.error)}</p>
            <Button variant="secondary" onClick={() => logs.refetch()}>
              Try again
            </Button>
          </div>
        </GlassCard>
      )}

      {logs.isSuccess && items.length === 0 && (
        <GlassCard>
          <div className={styles.empty}>
            {filtered ? (
              <>
                <p>No logs match these filters in the {range}.</p>
                <Button variant="secondary" onClick={() => setFilters({ range: search.range })}>
                  Clear filters
                </Button>
              </>
            ) : (
              <>
                <p>No logs in the {range} yet.</p>
                <Link to="/p/$projectId/connect" params={{ projectId }}>
                  Connect your app
                </Link>
              </>
            )}
          </div>
        </GlassCard>
      )}

      {items.length > 0 && <LogList logs={items} />}

      {logs.hasNextPage && (
        <div className={styles.more}>
          <Button variant="secondary" loading={logs.isFetchingNextPage} onClick={() => logs.fetchNextPage()}>
            {logs.isFetchingNextPage ? 'Loading…' : 'Load older'}
          </Button>
        </div>
      )}
      {logs.isSuccess && items.length > 0 && !logs.hasNextPage && (
        <p className={styles.end}>That's everything in the {range}.</p>
      )}
    </>
  )
}
