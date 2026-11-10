import { useState, type FormEvent } from 'react'
import { Button } from '../../components/ui/Button'
import { Modal } from '../../components/ui/Modal'
import { TextField } from '../../components/ui/TextField'
import { apiErrorMessage } from '../../lib/errors'
import type { CreatedProject, Environment } from '../../lib/types'
import { useCreateProject } from './queries'
import styles from './CreateProjectDialog.module.css'

/** Same rule as the server: 1–64 characters, starting with a letter or digit. */
const NAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}$/

const environments: { value: Environment; title: string; text: string; prefix: string }[] = [
  { value: 'live', title: 'Live', text: 'Real traffic. Key starts with', prefix: 'aidv_live_' },
  { value: 'test', title: 'Test', text: 'For trying things out. Key starts with', prefix: 'aidv_test_' },
]

interface CreateProjectDialogProps {
  onClose: () => void
  onCreated: (project: CreatedProject) => void
}

export function CreateProjectDialog({ onClose, onCreated }: CreateProjectDialogProps) {
  const create = useCreateProject()
  const [name, setName] = useState('')
  const [environment, setEnvironment] = useState<Environment>('live')
  const [nameError, setNameError] = useState<string>()

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) return setNameError('Enter a name for the project.')
    if (!NAME_PATTERN.test(trimmed)) {
      return setNameError('Use up to 64 letters, digits, spaces, - _ or . and start with a letter or digit.')
    }
    setNameError(undefined)
    create.mutate({ name: trimmed, environment }, { onSuccess: onCreated })
  }

  return (
    <Modal title="New project" onClose={onClose}>
      <form className={styles.form} onSubmit={onSubmit} noValidate>
        <TextField
          label="Name"
          name="name"
          autoComplete="off"
          placeholder="shop-backend"
          value={name}
          onChange={(e) => setName(e.target.value)}
          hint="Usually the name of your app or service. Letters, numbers, spaces, - _ ."
          error={nameError}
          data-autofocus
        />

        <fieldset className={styles.fieldset}>
          <legend className={styles.legend}>Environment</legend>
          {environments.map((env) => (
            <label
              key={env.value}
              className={environment === env.value ? `${styles.option} ${styles.selected}` : styles.option}
            >
              <input
                type="radio"
                name="environment"
                value={env.value}
                checked={environment === env.value}
                onChange={() => setEnvironment(env.value)}
                className={styles.radio}
              />
              <span className={styles.optionText}>
                <span className={styles.optionTitle}>{env.title}</span>
                <span className={styles.optionBody}>
                  {env.text} <code className={styles.prefix}>{env.prefix}</code>
                </span>
              </span>
            </label>
          ))}
        </fieldset>

        {create.isError && (
          <p role="alert" className={styles.error}>
            {apiErrorMessage(create.error, {
              project_exists: 'You already have a project with this name in this environment.',
            })}
          </p>
        )}

        <div className={styles.actions}>
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" loading={create.isPending}>
            {create.isPending ? 'Creating…' : 'Create project'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}
