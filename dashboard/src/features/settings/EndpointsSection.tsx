import { Link } from '@tanstack/react-router'
import { GlassCard } from '../../components/ui/GlassCard'
import { API_URL } from '../../lib/api'
import styles from './Settings.module.css'

/** Where apps send their data. */
export function EndpointsSection({ projectId }: { projectId: string }) {
  const rows = [
    { label: 'OpenTelemetry (OTLP/HTTP)', value: API_URL },
    { label: 'Custom JSON logs', value: `${API_URL}/api/v1/telemetry/logs` },
    { label: 'Custom JSON metrics', value: `${API_URL}/api/v1/telemetry/metrics` },
  ]
  return (
    <GlassCard as="section">
      <div className={styles.section}>
        <h2 className={styles.title}>Endpoints</h2>
        <dl className={styles.endpoints}>
          {rows.map((r) => (
            <div key={r.label}>
              <dt>{r.label}</dt>
              <dd className={styles.mono}>{r.value}</dd>
            </div>
          ))}
        </dl>
        <Link to="/p/$projectId/connect" params={{ projectId }} className={styles.inlineLink}>
          Setup instructions
        </Link>
      </div>
    </GlassCard>
  )
}
