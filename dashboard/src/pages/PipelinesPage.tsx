import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useEffect } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { Button } from '../components/ui/Button'
import { GlassCard } from '../components/ui/GlassCard'
import { CheckIcon, CloseIcon } from '../components/ui/icons/Icons'
import { SegmentedControl } from '../components/ui/SegmentedControl'
import { Skeleton } from '../components/ui/Skeleton'
import { StatCard } from '../components/ui/StatCard'
import { rangeLabels } from '../features/logs/filters'
import { RunDetail } from '../features/pipelines/RunDetail'
import { RESULTS, usePipelines, type ResultFilter } from '../features/pipelines/queries'
import styles from '../features/pipelines/Pipelines.module.css'
import { useNow } from '../hooks/useNow'
import { apiErrorMessage } from '../lib/errors'
import { timeAgo } from '../lib/format'

const resultLabels: Record<ResultFilter, string> = { all: 'All', passed: 'Passed', failed: 'Failed' }

/** Every push the CI checked: passed, or failed at which step and why. */
export function PipelinesPage() {
  const { projectId } = useParams({ from: '/shell/p/$projectId/pipelines' })
  const search = useSearch({ from: '/shell/p/$projectId/pipelines' })
  const navigate = useNavigate({ from: '/p/$projectId/pipelines' })
  const range = search.range ?? '7d'
  const runs = usePipelines(projectId, range)
  const now = useNow()

  useEffect(() => {
    document.title = 'Pipelines · AIDevOps'
  }, [])

  const all = runs.data ?? []
  const shown = search.result ? all.filter((r) => r.result === search.result) : all
  const explicit = all.find((r) => r.run_id === search.run)
  const selected = explicit ?? shown[0]
  const failedCount = all.filter((r) => r.result === 'failed').length
  const repos = new Set(all.map((r) => r.repository || r.service)).size
  const rangeText = rangeLabels[range].toLowerCase()

  return (
    <>
      <PageHeader
        title="Pipelines"
        subtitle="Every push, as reported by your CI. A failed run is also saved as an error."
        actions={
          <SegmentedControl
            label="Result"
            options={RESULTS}
            labels={resultLabels}
            value={search.result ?? 'all'}
            onChange={(v) => void navigate({ search: { range: search.range, result: v === 'all' ? undefined : v }, replace: true })}
          />
        }
      />

      {runs.isPending && (
        <GlassCard>
          <div className={styles.loading} aria-label="Loading pipeline runs">
            <Skeleton width="60%" />
            <Skeleton width="40%" />
          </div>
        </GlassCard>
      )}
      {runs.isError && !runs.data && (
        <GlassCard>
          <div role="alert" className={styles.empty}>
            <p className={styles.errorText}>{apiErrorMessage(runs.error)}</p>
            <Button variant="secondary" onClick={() => runs.refetch()}>
              Try again
            </Button>
          </div>
        </GlassCard>
      )}
      {runs.isSuccess && all.length === 0 && (
        <GlassCard>
          <div className={styles.empty}>
            <p>No pipeline runs in the {rangeText}.</p>
            <p>
              Runs appear here when your CI sends its result as a log with a <code>ci.run.id</code> attribute (plus{' '}
              <code>vcs.branch</code>, <code>vcs.commit.sha</code> and <code>ci.failed_step</code> on failures), using
              this project's API key.
            </p>
            <Link to="/p/$projectId/settings" params={{ projectId }}>
              See the endpoints
            </Link>
          </div>
        </GlassCard>
      )}

      {all.length > 0 && (
        <>
          <div className={styles.stats}>
            <StatCard label="Runs" value={all.length} note={`${repos} ${repos === 1 ? 'repository' : 'repositories'} · ${rangeText}`} />
            <StatCard label="Passed" value={all.length - failedCount} note="Ready to deploy" />
            <StatCard
              label="Failed"
              value={failedCount}
              tone={failedCount > 0 ? 'error' : 'default'}
              note={failedCount > 0 ? 'Blocked from deploying' : 'No failed pushes'}
            />
            <StatCard label="Last run" value={timeAgo(all[0].at, now)} note={all[0].repository || all[0].service} />
          </div>

          <div className={`${styles.layout} ${explicit ? styles.hasSelection : styles.noSelection}`}>
            <div className={styles.listCard}>
              {shown.length === 0 ? (
                <p className={`${styles.empty} ${styles.loading}`}>No {search.result} runs in the {rangeText}.</p>
              ) : (
                <ul className={styles.list} aria-label="Pipeline runs">
                  {shown.map((run) => {
                    const ok = run.result === 'passed'
                    const isSelected = run.run_id === selected?.run_id
                    return (
                      <li key={run.run_id}>
                        <Link
                          to="/p/$projectId/pipelines"
                          params={{ projectId }}
                          search={{ ...search, run: run.run_id }}
                          replace
                          className={isSelected ? `${styles.item} ${styles.selected}` : styles.item}
                          aria-current={isSelected ? 'true' : undefined}
                        >
                          <span className={`${styles.badge} ${ok ? styles.badgeOk : styles.badgeFail}`} aria-label={ok ? 'Passed' : 'Failed'}>
                            {ok ? <CheckIcon size={12} /> : <CloseIcon size={12} />}
                          </span>
                          <span className={styles.itemText}>
                            <span className={styles.itemTitle}>{run.commit_message || `${run.pipeline} run ${run.run_id}`}</span>
                            <span className={styles.itemMeta}>
                              <span className={styles.mono}>{run.commit_sha?.slice(0, 7)}</span>
                              {run.branch && ` · ${run.branch}`}
                              {run.author && ` · ${run.author}`}
                            </span>
                          </span>
                          <span className={styles.itemSide}>
                            <span>{timeAgo(run.at, now)}</span>
                            <span className={ok ? undefined : styles.stateFail}>
                              {ok ? 'Passed' : `Failed at ${run.failed_steps[0]?.name.toLowerCase() ?? 'a step'}`}
                            </span>
                          </span>
                        </Link>
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>
            {selected && <RunDetail projectId={projectId} run={selected} search={search} />}
          </div>
        </>
      )}
    </>
  )
}
