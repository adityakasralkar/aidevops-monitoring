import { Link } from '@tanstack/react-router'
import { CodeBlock } from '../../components/ui/CodeBlock'
import { CheckIcon } from '../../components/ui/icons/Icons'
import { Skeleton } from '../../components/ui/Skeleton'
import { useNow } from '../../hooks/useNow'
import { apiErrorMessage } from '../../lib/errors'
import { timeAgo } from '../../lib/format'
import { testLogCommand } from './languages'
import { languageName, useServices } from './queries'
import styles from './ConnectionPanel.module.css'

interface ConnectionPanelProps {
  projectId: string
  /** The program being connected (cleaned name). */
  service: string
  endpoint: string
}

/** Right side of Connect: did it work? Re-checks the server every 5 seconds. */
export function ConnectionPanel({ projectId, service, endpoint }: ConnectionPanelProps) {
  const services = useServices(projectId, { poll: true })
  const now = useNow(5_000)
  const match = services.data?.find((s) => s.name === service)

  return (
    <aside className={styles.panel} aria-label="Connection status">
      <section className={styles.card} aria-live="polite">
        {match ? (
          <>
            <div className={styles.statusRow}>
              <span className={`${styles.badge} ${styles.ok}`} aria-hidden="true">
                <CheckIcon size={12} />
              </span>
              <h2 className={styles.title}>{service} is connected</h2>
            </div>
            <p className={styles.text}>Last data {timeAgo(match.last_seen, now).toLowerCase()}. Its logs and metrics are now in this project.</p>
            <Link to="/p/$projectId" params={{ projectId }} className={styles.linkButton}>
              Open overview
            </Link>
          </>
        ) : (
          <>
            <div className={styles.statusRow}>
              <span className={styles.waitingDot} aria-hidden="true" />
              <h2 className={styles.title}>Waiting for {service}</h2>
            </div>
            <p className={styles.text}>It will show up here by itself as soon as its first data arrives.</p>
          </>
        )}

        <div className={styles.testLog}>
          <CodeBlock code={testLogCommand({ service, endpoint })} caption="Send a test log" copyLabel="Copy" wrap />
          <p className={styles.small}>Checks the key and endpoint without your app. Run it in a terminal; the log shows up in Logs.</p>
        </div>
      </section>

      <section className={styles.card} aria-labelledby="connected-title">
        <h2 id="connected-title" className={styles.title}>
          Connected so far
        </h2>
        {services.isPending && (
          <div className={styles.loading}>
            <Skeleton width="70%" />
            <Skeleton width="50%" />
          </div>
        )}
        {services.isError && <p className={styles.error}>{apiErrorMessage(services.error)}</p>}
        {services.isSuccess &&
          (services.data.length === 0 ? (
            <p className={styles.small}>Nothing has sent data in the last 24 hours.</p>
          ) : (
            <ul className={styles.services}>
              {services.data.map((s) => (
                <li key={s.name} className={styles.service}>
                  <span className={styles.serviceName}>{s.name}</span>
                  <span className={styles.serviceLang}>{languageName(s.language)}</span>
                </li>
              ))}
            </ul>
          ))}
      </section>

      <section className={styles.card} aria-labelledby="help-title">
        <h2 id="help-title" className={styles.title}>
          Not seeing data?
        </h2>
        <ul className={styles.help}>
          <li>
            <code>invalid_api_key</code> — the key is wrong or has an extra space.
          </li>
          <li>
            <code>env_mismatch</code> — a test key is used for a live project, or the other way round.
          </li>
          <li>
            Nothing at all — check that the protocol line is <code>http/protobuf</code> and the endpoint has no path at
            the end.
          </li>
          <li>
            Errors about traces — keep the <code>OTEL_TRACES_EXPORTER=none</code> line.
          </li>
        </ul>
      </section>
    </aside>
  )
}
