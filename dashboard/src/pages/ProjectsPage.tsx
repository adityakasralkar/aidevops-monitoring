import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { PageHeader } from '../components/layout/PageHeader'
import { TopBar } from '../components/layout/TopBar'
import { Button } from '../components/ui/Button'
import { GlassCard } from '../components/ui/GlassCard'
import { PlusIcon } from '../components/ui/icons/Icons'
import { Skeleton } from '../components/ui/Skeleton'
import { ApiKeyDialog } from '../features/projects/ApiKeyDialog'
import { CreateProjectDialog } from '../features/projects/CreateProjectDialog'
import { ProjectList } from '../features/projects/ProjectList'
import { ProjectsEmpty } from '../features/projects/ProjectsEmpty'
import { useProjects } from '../features/projects/queries'
import { apiErrorMessage } from '../lib/errors'
import type { CreatedProject } from '../lib/types'
import styles from './ProjectsPage.module.css'

type Dialog = { kind: 'create' } | { kind: 'key'; project: CreatedProject } | null

/** Your projects: list, first-visit guide, and creating a new project. */
export function ProjectsPage() {
  const navigate = useNavigate()
  const projects = useProjects()
  const [dialog, setDialog] = useState<Dialog>(null)

  useEffect(() => {
    document.title = 'Projects · AIDevOps'
  }, [])

  const openCreate = () => setDialog({ kind: 'create' })
  const count = projects.data?.length ?? 0
  const hasProjects = count > 0

  return (
    <div className={styles.page}>
      <TopBar />
      <main className={styles.main}>
        {(projects.isPending || projects.isError || hasProjects) && (
          <PageHeader
            title="Projects"
            subtitle={hasProjects ? `${count} ${count === 1 ? 'project' : 'projects'}` : undefined}
            actions={
              hasProjects && (
                <Button onClick={openCreate}>
                  <PlusIcon />
                  New project
                </Button>
              )
            }
          />
        )}

        {projects.isPending && (
          <GlassCard>
            <div className={styles.loading} aria-label="Loading projects">
              <Skeleton width="40%" />
              <Skeleton width="65%" />
              <Skeleton width="50%" />
            </div>
          </GlassCard>
        )}

        {projects.isError && (
          <GlassCard>
            <div role="alert" className={styles.error}>
              <p>{apiErrorMessage(projects.error)}</p>
              <Button variant="secondary" onClick={() => projects.refetch()}>
                Try again
              </Button>
            </div>
          </GlassCard>
        )}

        {projects.isSuccess && (hasProjects ? <ProjectList projects={projects.data} /> : <ProjectsEmpty onCreate={openCreate} />)}
      </main>

      {dialog?.kind === 'create' && (
        <CreateProjectDialog onClose={() => setDialog(null)} onCreated={(project) => setDialog({ kind: 'key', project })} />
      )}
      {dialog?.kind === 'key' && (
        <ApiKeyDialog
          project={dialog.project}
          onContinue={() => {
            const id = dialog.project.id
            setDialog(null)
            void navigate({ to: '/p/$projectId/connect', params: { projectId: id } })
          }}
        />
      )}
    </div>
  )
}
