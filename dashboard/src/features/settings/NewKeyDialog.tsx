import { useEffect } from 'react'
import { Button } from '../../components/ui/Button'
import { CopyButton } from '../../components/ui/CopyButton'
import { WarningIcon } from '../../components/ui/icons/Icons'
import { Modal } from '../../components/ui/Modal'
import { apiErrorMessage } from '../../lib/errors'
import type { Project } from '../../lib/types'
import { useRotateKey } from './queries'
import styles from './Settings.module.css'

/**
 * Two steps: confirm (the old key stops working), then the new key, shown
 * once. While the key is on screen the popup can't be closed by accident.
 */
export function NewKeyDialog({ project, onClose }: { project: Project; onClose: () => void }) {
  const rotate = useRotateKey(project.id)
  const created = rotate.data

  useEffect(() => {
    if (!created) return
    const warn = (event: BeforeUnloadEvent) => event.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [created])

  if (created) {
    return (
      <Modal title="Your new API key" onClose={onClose} dismissible={false} size="lg">
        <p className={styles.lead}>The old key no longer works. Put this one in your app now.</p>
        <div className={styles.keyBox}>
          <code className={styles.key}>{created.api_key}</code>
          <CopyButton text={created.api_key} label="Copy API key" />
        </div>
        <div className={styles.warning} role="note">
          <WarningIcon className={styles.warningIcon} />
          <span>
            <strong>You won't see this key again.</strong> Replace the old key in{' '}
            <code>OTEL_EXPORTER_OTLP_HEADERS</code> and restart your app.
          </span>
        </div>
        <div className={styles.actions}>
          <Button onClick={onClose}>Done</Button>
        </div>
      </Modal>
    )
  }

  return (
    <Modal title="Create a new API key?" onClose={onClose}>
      <p className={styles.lead}>Use this if the current key was lost or may have leaked.</p>
      <div className={styles.warning} role="note">
        <WarningIcon className={styles.warningIcon} />
        <span>
          The current key <code>{project.key_prefix}…</code> stops working straight away. Data from your app will be
          refused until you update <code>OTEL_EXPORTER_OTLP_HEADERS</code>.
        </span>
      </div>
      {rotate.isError && (
        <p role="alert" className={styles.formError}>
          {apiErrorMessage(rotate.error)}
        </p>
      )}
      <div className={styles.actions}>
        <Button variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button onClick={() => rotate.mutate()} loading={rotate.isPending}>
          {rotate.isPending ? 'Creating…' : 'Create new key'}
        </Button>
      </div>
    </Modal>
  )
}
