import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useEffect } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { Button } from '../components/ui/Button'
import { GlassCard } from '../components/ui/GlassCard'
import { Select } from '../components/ui/Select'
import { Skeleton } from '../components/ui/Skeleton'
import { useServices } from '../features/connect/queries'
import { ErrorDetailView } from '../features/errors/ErrorDetailView'
import { ErrorList } from '../features/errors/ErrorList'
import { DEFAULT_ERROR_RANGE, type ErrorSearch } from '../features/errors/filters'
import { useErrors } from '../features/errors/queries'
import styles from '../features/errors/Errors.module.css'
import { rangeLabels } from '../features/logs/filters'
import { RANGES, type Range } from '../features/overview/range'
import { apiErrorMessage } from '../lib/errors'
import type { DiagnosisStatus } from '../lib/types'

const statusOptions = [
  { value: '', label: 'Any diagnosis' },
  { value: 'PENDING', label: 'AI pending' },
  { value: 'COMPLETED', label: 'Diagnosed' },
  { value: 'FAILED', label: 'Diagnosis failed' },
]

/** ERROR and CRITICAL logs, each with its context and AI diagnosis. */
export function ErrorsPage() {
  const { projectId } = useParams({ from: '/shell/p/$projectId/errors' })
  const search = useSearch({ from: '/shell/p/$projectId/errors' })
  const navigate = useNavigate({ from: '/p/$projectId/errors' })
  const range = search.range ?? DEFAULT_ERROR_RANGE

  const errors = useErrors(projectId, { range, status: search.status, service: search.service })
  const services = useServices(projectId)
  const items = errors.data?.pages.flatMap((p) => p.items) ?? []
  // Desktop shows the first error when none is picked; the URL keeps an explicit pick.
  const selectedId = search.id ?? items[0]?.id

  useEffect(() => {
    document.title = 'Errors · AIDevOps'
  }, [])

  // Changing a filter drops the selected error (it may not be in the new list).
  const setFilters = (next: ErrorSearch) => void navigate({ search: { ...next, id: undefined }, replace: true })
  const rangeText = rangeLabels[range].toLowerCase()

  return (
    <>
      <PageHeader
        title="Errors"
        subtitle={`ERROR and CRITICAL logs from the ${rangeText}. Each one is saved as PENDING for AI diagnosis.`}
      />

      <div className={`${styles.layout} ${search.id ? styles.hasSelection : styles.noSelection}`}>
        <div className={styles.side}>
          <div className={styles.filters}>
            <Select
              label="Time range"
              options={RANGES.map((r) => ({ value: r, label: rangeLabels[r] }))}
              value={range}
              onChange={(e) => setFilters({ ...search, range: e.target.value as Range })}
            />
            <Select
              label="Service"
              options={[{ value: '', label: 'All services' }, ...(services.data ?? []).map((s) => ({ value: s.name, label: s.name }))]}
              value={search.service ?? ''}
              onChange={(e) => setFilters({ ...search, service: e.target.value || undefined })}
            />
            <Select
              label="Diagnosis"
              options={statusOptions}
              value={search.status ?? ''}
              onChange={(e) =>
                setFilters({ ...search, status: (e.target.value || undefined) as DiagnosisStatus | undefined })
              }
            />
          </div>

          {errors.isPending && (
            <GlassCard>
              <div className={styles.loading} aria-label="Loading errors">
                <Skeleton width="80%" />
                <Skeleton width="60%" />
                <Skeleton width="70%" />
              </div>
            </GlassCard>
          )}
          {errors.isError && !errors.data && (
            <GlassCard>
              <div role="alert" className={styles.empty}>
                <p className={styles.errorText}>{apiErrorMessage(errors.error)}</p>
                <Button variant="secondary" onClick={() => errors.refetch()}>
                  Try again
                </Button>
              </div>
            </GlassCard>
          )}
          {errors.isSuccess && items.length === 0 && (
            <GlassCard>
              <div className={styles.empty}>
                <p>No errors in the {rangeText}{search.service || search.status ? ' with these filters' : ''}.</p>
              </div>
            </GlassCard>
          )}
          {items.length > 0 && (
            <div className={styles.listCard}>
              <ErrorList projectId={projectId} errors={items} selectedId={selectedId} search={search} />
            </div>
          )}
          {errors.hasNextPage && (
            <div className={styles.more}>
              <Button variant="secondary" loading={errors.isFetchingNextPage} onClick={() => errors.fetchNextPage()}>
                {errors.isFetchingNextPage ? 'Loading…' : 'Load older'}
              </Button>
            </div>
          )}
        </div>

        <ErrorDetailView projectId={projectId} errorId={selectedId} search={search} />
      </div>
    </>
  )
}
