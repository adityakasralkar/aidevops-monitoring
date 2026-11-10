import { Button } from '../../components/ui/Button'
import { GlassCard } from '../../components/ui/GlassCard'
import { useNow } from '../../hooks/useNow'
import { dateOnly, timeAgo } from '../../lib/format'
import type { Project } from '../../lib/types'
import styles from './Settings.module.css'

/** Shows only the key's start; the full key can't be shown again. */
export function ApiKeySection({ project, onNewKey }: { project: Project; onNewKey: () => void }) {
  const now = useNow()
  return (
    <GlassCard as="section">
      <div className={styles.section}>
        <h2 className={styles.title}>API key</h2>
        <p className={styles.text}>
          Only the start of the key is stored for display. The full key was shown once, when it was created.
        </p>
        <div className={styles.keyRow}>
          <div className={styles.keyInfo}>
            <code className={styles.keyPrefix}>{project.key_prefix}••••••••••••</code>
            <span className={styles.small}>
              Created {dateOnly(project.key_created_at)} · last used{' '}
              {project.last_used_at ? timeAgo(project.last_used_at, now).toLowerCase() : 'never'}
            </span>
          </div>
          <Button variant="secondary" onClick={onNewKey}>
            Create new key
          </Button>
        </div>
      </div>
    </GlassCard>
  )
}
