import { useEffect, useState } from 'react'
import { Button } from '../../components/ui/Button'
import { CopyButton } from '../../components/ui/CopyButton'
import { WarningIcon } from '../../components/ui/icons/Icons'
import { Modal } from '../../components/ui/Modal'
import type { CreatedProject } from '../../lib/types'
import styles from './ApiKeyDialog.module.css'

interface ApiKeyDialogProps {
  project: CreatedProject
  onContinue: () => void
}

/**
 * Shows a new project's API key, the only time it is ever visible.
 * It can't be closed by accident, and leaving the page asks first.
 */
export function ApiKeyDialog({ project, onContinue }: ApiKeyDialogProps) {
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => event.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [])

  return (
    <Modal title="Save your API key" onClose={onContinue} dismissible={false} size="lg">
      <p className={styles.lead}>
        <strong>{project.name}</strong> was created. Your app sends this key with every request.
      </p>

      <div className={styles.keyField}>
        <span className={styles.keyLabel} id="api-key-label">
          API key
        </span>
        <div className={styles.keyBox}>
          <code className={styles.key} aria-labelledby="api-key-label">
            {project.api_key}
          </code>
          <CopyButton text={project.api_key} label="Copy API key" />
        </div>
      </div>

      <div className={styles.warning} role="note">
        <WarningIcon className={styles.warningIcon} />
        <span>
          <strong>You won't see this key again.</strong> Only a fingerprint of it is stored. If you lose it, create a
          new key in Settings.
        </span>
      </div>

      <label className={styles.confirm}>
        <input type="checkbox" checked={saved} onChange={(e) => setSaved(e.target.checked)} />
        <span>I have saved this key</span>
      </label>

      <div className={styles.actions}>
        <Button onClick={onContinue} disabled={!saved}>
          Continue
        </Button>
      </div>
    </Modal>
  )
}
