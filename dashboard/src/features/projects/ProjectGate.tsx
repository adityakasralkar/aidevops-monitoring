import { Link, Outlet, useParams } from '@tanstack/react-router'
import { PageHeader } from '../../components/layout/PageHeader'
import { GlassCard } from '../../components/ui/GlassCard'
import { ApiError } from '../../lib/api'
import { useProject } from './queries'
import styles from './ProjectGate.module.css'

/**
 * Wraps a project's pages. If the project doesn't exist or belongs to
 * someone else (the server answers 404 for both), say so instead of
 * showing empty pages.
 */
export function ProjectGate() {
  const { projectId } = useParams({ from: '/shell/p/$projectId' })
  const project = useProject(projectId)

  if (project.error instanceof ApiError && project.error.status === 404) {
    return (
      <>
        <PageHeader title="Project not found" />
        <GlassCard>
          <div className={styles.body}>
            <p>This project doesn't exist, or it isn't one of yours.</p>
            <Link to="/projects">Go to your projects</Link>
          </div>
        </GlassCard>
      </>
    )
  }
  return <Outlet />
}
