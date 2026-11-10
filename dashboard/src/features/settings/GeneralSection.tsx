import { useState, type FormEvent } from 'react'
import { Button } from '../../components/ui/Button'
import { GlassCard } from '../../components/ui/GlassCard'
import { TextField } from '../../components/ui/TextField'
import { apiErrorMessage } from '../../lib/errors'
import { dateOnly } from '../../lib/format'
import type { Project } from '../../lib/types'
import { useRenameProject } from './queries'
import styles from './Settings.module.css'

/** Same rule as the server and the create form. */
const NAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}$/

/** Project name (editable), environment, created date and ID. */
export function GeneralSection({ project }: { project: Project }) {
  const rename = useRenameProject(project.id)
  const [name, setName] = useState(project.name)
  const [nameError, setNameError] = useState<string>()
  const changed = name.trim() !== project.name

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    const trimmed = name.trim()
    if (!NAME_PATTERN.test(trimmed)) {
      return setNameError('Use up to 64 letters, digits, spaces, - _ or . and start with a letter or digit.')
    }
    setNameError(undefined)
    rename.mutate(trimmed)
  }

  return (
    <GlassCard as="section">
      <form className={styles.section} onSubmit={onSubmit} noValidate>
        <h2 className={styles.title}>General</h2>
        <div className={styles.nameRow}>
          <TextField
            label="Project name"
            value={name}
            onChange={(e) => {
              setName(e.target.value)
              rename.reset()
            }}
            error={nameError}
            autoComplete="off"
          />
        </div>
        <dl className={styles.facts}>
          <div>
            <dt>Environment</dt>
            <dd>{project.environment === 'live' ? 'Live' : 'Test'} (fixed)</dd>
          </div>
          <div>
            <dt>Created</dt>
            <dd>{dateOnly(project.created_at)}</dd>
          </div>
          <div>
            <dt>Project ID</dt>
            <dd className={styles.mono}>{project.id}</dd>
          </div>
        </dl>
        {rename.isError && (
          <p role="alert" className={styles.formError}>
            {apiErrorMessage(rename.error, {
              project_exists: 'You already have a project with this name in this environment.',
            })}
          </p>
        )}
        <div className={styles.saveRow}>
          {rename.isSuccess && !changed && (
            <span role="status" className={styles.saved}>
              Saved
            </span>
          )}
          <Button type="submit" disabled={!changed} loading={rename.isPending}>
            {rename.isPending ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </form>
    </GlassCard>
  )
}
