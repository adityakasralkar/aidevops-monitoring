import { Button } from '../../components/ui/Button'
import { PlusIcon } from '../../components/ui/icons/Icons'
import styles from './ProjectsEmpty.module.css'

const steps = [
  { title: 'Create a project', text: 'Name it and choose live or test.' },
  { title: 'Save the API key', text: 'It is shown once. Keep it like a password.' },
  { title: 'Connect your app', text: 'Add two OpenTelemetry settings and run it.' },
]

/** First visit: explains what a project is and how to get data in. */
export function ProjectsEmpty({ onCreate }: { onCreate: () => void }) {
  return (
    <section className={styles.card} aria-labelledby="first-project-title">
      <div className={styles.intro}>
        <h1 id="first-project-title" className={styles.title}>
          Create your first project
        </h1>
        <p className={styles.text}>
          A project is one app you want to monitor. Each project gets its own API key, so its logs and metrics stay
          separate.
        </p>
      </div>

      <ol className={styles.steps}>
        {steps.map((step, i) => (
          <li key={step.title} className={styles.step}>
            <span className={styles.number}>{i + 1}</span>
            <span className={styles.stepText}>
              <span className={styles.stepTitle}>{step.title}</span>
              <span className={styles.stepBody}>{step.text}</span>
            </span>
          </li>
        ))}
      </ol>

      <Button className={styles.cta} onClick={onCreate}>
        <PlusIcon />
        Create project
      </Button>
    </section>
  )
}
