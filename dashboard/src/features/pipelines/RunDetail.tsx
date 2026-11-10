import { Link } from '@tanstack/react-router'
import { GlassCard } from '../../components/ui/GlassCard'
import { ChevronRightIcon, ExternalIcon } from '../../components/ui/icons/Icons'
import { SeverityPill } from '../../components/ui/SeverityPill'
import { dateTime } from '../../lib/format'
import type { PipelineRun } from '../../lib/types'
import type { PipelineSearch } from './queries'
import styles from './Pipelines.module.css'

/** Everything about one run: the push, and for a failure, what broke. */
export function RunDetail({ projectId, run, search }: { projectId: string; run: PipelineRun; search: PipelineSearch }) {
  const { run: _r, ...listSearch } = search
  const failed = run.result === 'failed'
  const facts = [
    { label: 'Repository', value: run.repository || run.service },
    { label: 'Branch', value: run.branch, mono: true },
    { label: 'Commit', value: run.commit_sha?.slice(0, 7), mono: true },
    { label: 'Pushed by', value: run.author },
    { label: 'Finished', value: dateTime(run.at) },
  ].filter((f) => f.value)

  return (
    <div className={styles.detail}>
      <Link to="/p/$projectId/pipelines" params={{ projectId }} search={listSearch} className={styles.back}>
        <ChevronRightIcon size={14} className={styles.backIcon} />
        All runs
      </Link>
      <GlassCard as="section">
        <div className={styles.detailHead}>
          <span className={failed ? `${styles.result} ${styles.failed}` : `${styles.result} ${styles.passed}`}>
            {failed ? `Failed at ${run.failed_steps[0]?.name.toLowerCase() ?? 'a step'}` : 'Passed · ready to deploy'}
          </span>
          {run.url && (
            <a href={run.url} target="_blank" rel="noreferrer" className={styles.external}>
              Open run on GitHub
              <ExternalIcon size={13} />
            </a>
          )}
        </div>
        <h2 className={styles.commitMessage}>{run.commit_message || `${run.pipeline} run ${run.run_id}`}</h2>
        <dl className={styles.facts}>
          {facts.map((f) => (
            <div key={f.label}>
              <dt>{f.label}</dt>
              <dd className={f.mono ? styles.mono : undefined}>{f.value}</dd>
            </div>
          ))}
        </dl>
      </GlassCard>

      {failed &&
        run.failed_steps.map((step) => (
          <GlassCard as="section" key={step.log_id} className={styles.failCard}>
            <div className={styles.failHead}>
              <div className={styles.failTitle}>
                <SeverityPill severity={step.severity} />
                <h3>{step.name}</h3>
              </div>
              <Link to="/p/$projectId/errors" params={{ projectId }} search={{ id: step.log_id, range: '7d' }} className={styles.external}>
                View as error
              </Link>
            </div>
            <p className={styles.failMessage}>{step.message}</p>
            {step.output && (
              <pre className={styles.output} tabIndex={0}>
                {step.output}
              </pre>
            )}
          </GlassCard>
        ))}
    </div>
  )
}
