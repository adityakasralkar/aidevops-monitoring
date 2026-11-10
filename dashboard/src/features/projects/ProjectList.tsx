import { Link } from '@tanstack/react-router'
import { StatusDot } from '../../components/ui/StatusDot'
import { useNow } from '../../hooks/useNow'
import { timeAgo } from '../../lib/format'
import type { Project } from '../../lib/types'
import { projectStatus } from './status'
import styles from './ProjectList.module.css'

/** Table of projects; each row opens the project. Stacks into cards on phones. */
export function ProjectList({ projects }: { projects: Project[] }) {
  const now = useNow()
  return (
    <div className={styles.card}>
      <div className={styles.head} aria-hidden="true">
        <span>Name</span>
        <span>Environment</span>
        <span>Status</span>
        <span>Last data</span>
      </div>
      <ul className={styles.rows}>
        {projects.map((project) => {
          const status = projectStatus(project, now)
          return (
            <li key={project.id}>
              <Link to="/p/$projectId" params={{ projectId: project.id }} className={styles.row}>
                <span className={styles.name}>{project.name}</span>
                <span className={styles.env}>
                  <span className={styles.label}>Environment: </span>
                  {project.environment}
                </span>
                <span className={styles.status}>
                  <StatusDot tone={status.tone} />
                  {status.text}
                </span>
                <span className={styles.last}>
                  <span className={styles.label}>Last data: </span>
                  {timeAgo(project.last_used_at, now)}
                </span>
              </Link>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
