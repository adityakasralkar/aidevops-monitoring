import { useParams } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { Button } from '../components/ui/Button'
import { GlassCard } from '../components/ui/GlassCard'
import { Skeleton } from '../components/ui/Skeleton'
import { useProject } from '../features/projects/queries'
import { ApiKeySection } from '../features/settings/ApiKeySection'
import { DeleteProjectDialog } from '../features/settings/DeleteProjectDialog'
import { EndpointsSection } from '../features/settings/EndpointsSection'
import { GeneralSection } from '../features/settings/GeneralSection'
import { NewKeyDialog } from '../features/settings/NewKeyDialog'
import styles from '../features/settings/Settings.module.css'

/** Project settings: name, API key, endpoints, delete. */
export function SettingsPage() {
  const { projectId } = useParams({ from: '/shell/p/$projectId/settings' })
  const project = useProject(projectId)
  const [dialog, setDialog] = useState<'key' | 'delete' | null>(null)

  useEffect(() => {
    document.title = 'Settings · AIDevOps'
  }, [])

  const p = project.data
  return (
    <>
      <PageHeader title="Settings" />
      <div className={styles.column}>
        {!p ? (
          <GlassCard>
            <div className={styles.loading} aria-label="Loading settings">
              <Skeleton width="40%" />
              <Skeleton width="70%" />
            </div>
          </GlassCard>
        ) : (
          <>
            <GeneralSection project={p} />
            <ApiKeySection project={p} onNewKey={() => setDialog('key')} />
            <EndpointsSection projectId={projectId} />
            <GlassCard as="section" className={styles.dangerCard}>
              <div className={styles.dangerRow}>
                <div className={styles.section}>
                  <h2 className={styles.dangerTitle}>Delete project</h2>
                  <p className={styles.text}>
                    Removes the project, its API key, and all its logs, metrics and diagnoses. This can't be undone.
                  </p>
                </div>
                <Button className={styles.danger} onClick={() => setDialog('delete')}>
                  Delete project
                </Button>
              </div>
            </GlassCard>
          </>
        )}
      </div>

      {p && dialog === 'key' && <NewKeyDialog project={p} onClose={() => setDialog(null)} />}
      {p && dialog === 'delete' && <DeleteProjectDialog project={p} onClose={() => setDialog(null)} />}
    </>
  )
}
