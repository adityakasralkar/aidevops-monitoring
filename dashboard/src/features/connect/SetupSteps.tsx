import { CodeBlock } from '../../components/ui/CodeBlock'
import { ExternalIcon } from '../../components/ui/icons/Icons'
import { TextField } from '../../components/ui/TextField'
import { languages, otelSettings, type LanguageId } from './languages'
import styles from './SetupSteps.module.css'

interface SetupStepsProps {
  language: LanguageId
  onLanguage: (id: LanguageId) => void
  serviceName: string
  onServiceName: (name: string) => void
  /** Name used in the commands (cleaned, never empty). */
  service: string
  endpoint: string
  keyPrefix?: string
}

/** The four steps to connect one program: language, install, settings, run. */
export function SetupSteps(props: SetupStepsProps) {
  const lang = languages.find((l) => l.id === props.language) ?? languages[0]

  return (
    <div className={styles.steps}>
      <section className={styles.card} aria-labelledby="step-language">
        <h2 id="step-language" className={styles.title}>
          <span className={styles.number}>1</span>Pick your language
        </h2>
        <div role="group" aria-labelledby="step-language" className={styles.languages}>
          {languages.map((l) => (
            <button
              key={l.id}
              type="button"
              aria-pressed={l.id === lang.id}
              className={styles.language}
              onClick={() => props.onLanguage(l.id)}
            >
              {l.label}
            </button>
          ))}
        </div>
      </section>

      <section className={styles.card} aria-labelledby="step-install">
        <h2 id="step-install" className={styles.title}>
          <span className={styles.number}>2</span>
          {lang.installTitle}
        </h2>
        {lang.id === 'other' ? (
          <div className={styles.other}>
            <p>
              Your language isn't listed here, but it still works. Open the official guide, find your language, and
              install its OpenTelemetry SDK. Then use the settings in step 3.
            </p>
            <a className={styles.linkButton} href="https://opentelemetry.io/docs/languages/" target="_blank" rel="noreferrer">
              OpenTelemetry guide for all languages
              <ExternalIcon size={14} />
            </a>
          </div>
        ) : (
          <CodeBlock code={lang.install} />
        )}
      </section>

      <section className={styles.card} aria-labelledby="step-settings">
        <h2 id="step-settings" className={styles.title}>
          <span className={styles.number}>3</span>Add these settings to your app
        </h2>
        <div className={styles.nameField}>
          <TextField
            label="Name of this program"
            value={props.serviceName}
            onChange={(e) => props.onServiceName(e.target.value)}
            placeholder="order-api"
            hint="Example: order-api. Use a different name for each program."
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <CodeBlock
          code={otelSettings({ service: props.service, endpoint: props.endpoint })}
          caption="Same for every language"
          copyLabel="Copy all settings"
        />
        <p className={styles.keyNote}>
          Replace <code>PASTE_YOUR_API_KEY</code> with the key you saved
          {props.keyPrefix && (
            <>
              {' '}
              (it starts with <code>{props.keyPrefix}</code>)
            </>
          )}
          . Lost it? Create a new key in Settings.
        </p>
        <ul className={styles.legend}>
          <li>
            <strong>Name</strong> — which program sent the data
          </li>
          <li>
            <strong>Endpoint</strong> — where this server runs
          </li>
          <li>
            <strong>API key</strong> — which project the data belongs to (same for all programs)
          </li>
          <li>
            <strong>Protocol</strong> — tells the SDK to send over HTTP
          </li>
          <li>
            <strong>Traces off</strong> — this server stores logs and metrics only
          </li>
        </ul>
      </section>

      <section className={styles.card} aria-labelledby="step-run">
        <h2 id="step-run" className={styles.title}>
          <span className={styles.number}>4</span>Run your app
        </h2>
        <CodeBlock code={lang.run} />
      </section>
    </div>
  )
}
