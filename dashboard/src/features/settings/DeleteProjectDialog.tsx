import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'
import { Button } from '../../components/ui/Button'
import { Modal } from '../../components/ui/Modal'
import { TextField } from '../../components/ui/TextField'
import { apiErrorMessage } from '../../lib/errors'
import type { Project } from '../../lib/types'
import { projectKeys } from '../projects/queries'
import { useDeleteProject } from './queries'
import styles from './Settings.module.css'

/** Deleting can't be undone, so the project's name must be typed first. */
export function DeleteProjectDialog({ project, onClose }: { project: Project; onClose: () => void }) {
  const remove = useDeleteProject(project.id)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [typed, setTyped] = useState('')
  const matches = typed.trim() === project.name

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (!matches) return
    remove.mutate(undefined, {
      onSuccess: async () => {
        await navigate({ to: '/projects', replace: true })
        // Forget everything cached for the deleted project.
        queryClient.removeQueries({ predicate: (q) => q.queryKey.includes(project.id) })
        void queryClient.invalidateQueries({ queryKey: projectKeys.all })
      },
    })
  }

  return (
    <Modal title={`Delete ${project.name}?`} onClose={onClose}>
      <form className={styles.form} onSubmit={onSubmit} noValidate>
        <div className={styles.lead}>
          <p>This permanently removes:</p>
          <ul className={styles.removes}>
            <li>the project and its API key, so your app's data will be refused</li>
            <li>all its logs, metrics and errors</li>
            <li>every AI diagnosis made for it</li>
          </ul>
        </div>
        <TextField
          label={`Type ${project.name} to confirm`}
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          autoComplete="off"
          spellCheck={false}
          data-autofocus
        />
        {remove.isError && (
          <p role="alert" className={styles.formError}>
            {apiErrorMessage(remove.error)}
          </p>
        )}
        <div className={styles.actions}>
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" className={styles.danger} disabled={!matches} loading={remove.isPending}>
            {remove.isPending ? 'Deleting…' : 'Delete project'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}
