import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { Button } from '../components/ui/Button'
import { GlassCard } from '../components/ui/GlassCard'
import { SearchIcon } from '../components/ui/icons/Icons'
import { SegmentedControl } from '../components/ui/SegmentedControl'
import { Skeleton } from '../components/ui/Skeleton'
import { rangeLabels } from '../features/logs/filters'
import { MetricDetail } from '../features/metrics/MetricDetail'
import { MetricList } from '../features/metrics/MetricList'
import { useMetricCatalog } from '../features/metrics/queries'
import styles from '../features/metrics/Metrics.module.css'
import { DEFAULT_RANGE, RANGES } from '../features/overview/range'
import { apiErrorMessage } from '../lib/errors'

/** Every metric the project's apps send, including custom ones. */
export function MetricsPage() {
  const { projectId } = useParams({ from: '/shell/p/$projectId/metrics' })
  const search = useSearch({ from: '/shell/p/$projectId/metrics' })
  const navigate = useNavigate({ from: '/p/$projectId/metrics' })
  const range = search.range ?? DEFAULT_RANGE
  const catalog = useMetricCatalog(projectId, range)
  const [filter, setFilter] = useState('')

  useEffect(() => {
    document.title = 'Metrics · AIDevOps'
  }, [])

  const all = catalog.data ?? []
  const needle = filter.trim().toLowerCase()
  const shown = needle ? all.filter((m) => `${m.name} ${m.service}`.toLowerCase().includes(needle)) : all
  const explicit = all.find((m) => m.name === search.name && m.service === search.service)
  // Desktop opens the first metric when none is picked.
  const selected = explicit ?? shown[0]
  const rangeText = rangeLabels[range].toLowerCase()

  return (
    <>
      <PageHeader
        title="Metrics"
        subtitle={`Everything your apps send, including custom metrics · ${rangeText}`}
        actions={
          <SegmentedControl
            label="Time range"
            options={RANGES}
            value={range}
            onChange={(next) => void navigate({ search: { ...search, range: next }, replace: true })}
          />
        }
      />

      {catalog.isPending && (
        <GlassCard>
          <div className={styles.loading} aria-label="Loading metrics">
            <Skeleton width="60%" />
          </div>
        </GlassCard>
      )}
      {catalog.isError && !catalog.data && (
        <GlassCard>
          <div role="alert" className={styles.empty}>
            <p className={styles.errorText}>{apiErrorMessage(catalog.error)}</p>
            <Button variant="secondary" onClick={() => catalog.refetch()}>
              Try again
            </Button>
          </div>
        </GlassCard>
      )}
      {catalog.isSuccess && all.length === 0 && (
        <GlassCard>
          <div className={styles.empty}>
            <p>No metrics in the {rangeText}.</p>
            <Link to="/p/$projectId/connect" params={{ projectId }}>
              Connect your app
            </Link>
          </div>
        </GlassCard>
      )}

      {all.length > 0 && (
        <div className={`${styles.layout} ${explicit ? styles.hasSelection : styles.noSelection}`}>
          <div className={styles.side}>
            <div className={styles.search}>
              <SearchIcon className={styles.searchIcon} />
              <label htmlFor="metric-filter" className="sr-only">
                Find a metric
              </label>
              <input
                id="metric-filter"
                type="search"
                className={styles.searchInput}
                placeholder={`Find among ${all.length} metrics`}
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
              />
            </div>
            <div className={styles.listCard}>
              {shown.length === 0 ? (
                <p className={`${styles.small} ${styles.noMatch}`}>
                  No metric matches "{filter}".
                </p>
              ) : (
                <MetricList projectId={projectId} metrics={shown} selected={selected} search={search} />
              )}
            </div>
          </div>
          <MetricDetail projectId={projectId} metric={selected} range={range} search={search} />
        </div>
      )}
    </>
  )
}
