import { Brand } from '../../components/layout/Brand'
import { ConnectIcon, DiagnoseIcon, ServerIcon } from '../../components/ui/icons/Icons'
import styles from './BrandPanel.module.css'

const features = [
  {
    Icon: ConnectIcon,
    title: 'Connect in minutes',
    text: 'Any language with an OpenTelemetry SDK. One API key, no code changes.',
  },
  {
    Icon: DiagnoseIcon,
    title: 'Errors explained',
    text: 'Each critical error is read with the metrics around it, then given a root cause and a fix.',
  },
  {
    Icon: ServerIcon,
    title: 'Runs on your server',
    text: 'Your telemetry is stored in your own database and never leaves it.',
  },
]

interface BrandPanelProps {
  /** After a session expired: reassure that no data was lost. */
  expired?: boolean
}

/** Dark left half of the login page: what AIDevOps is, in one glance. */
export function BrandPanel({ expired = false }: BrandPanelProps) {
  return (
    <section className={styles.panel} aria-label="About AIDevOps">
      <div className={styles.top}>
        <Brand tone="dark" />
        <span className={styles.chip}>Self-hosted</span>
      </div>

      <div className={styles.body}>
        <div className={styles.intro}>
          <span className={styles.kicker}>Monitoring for apps that use OpenTelemetry</span>
          <h1 className={styles.headline}>
            Plug in your app.
            <br />
            Know why it broke.
          </h1>
          <p className={styles.lead}>
            {expired
              ? 'Your projects kept receiving data while you were away. Nothing was lost.'
              : 'Collect logs and metrics from any service, and get a clear root cause for every serious error.'}
          </p>
        </div>

        <ul className={styles.features}>
          {features.map(({ Icon, title, text }) => (
            <li key={title} className={styles.feature}>
              <span className={styles.featureIcon}>
                <Icon />
              </span>
              <span className={styles.featureText}>
                <span className={styles.featureTitle}>{title}</span>
                <span className={styles.featureBody}>{text}</span>
              </span>
            </li>
          ))}
        </ul>

        <figure className={styles.example} aria-label="Example of an explained error">
          <div className={styles.exampleHead}>
            <span className={styles.exampleTag}>
              <span className={styles.critical}>CRITICAL</span>
              <span className={styles.mono}>order-api</span>
            </span>
            <span className={styles.monoMuted}>14:02:11</span>
          </div>
          <div className={styles.exampleMessage}>connection pool exhausted: timed out after 5000 ms</div>
          <figcaption className={styles.exampleAnswer}>
            <span className={styles.aiBadge}>AI</span>
            Pool limit of 10 was reached during a traffic spike. Add an index to the slow orders query and raise the
            limit to 25.
          </figcaption>
        </figure>
      </div>

      <div className={styles.footer}>
        <span>© 2026 AIDevOps</span>
        <span>OTLP/HTTP · Custom JSON · MongoDB</span>
      </div>
    </section>
  )
}
