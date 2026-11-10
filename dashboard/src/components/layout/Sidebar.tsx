import { Link, useParams } from '@tanstack/react-router'
import {
  ChevronsUpDownIcon,
  ConnectIcon,
  ErrorsIcon,
  LogsIcon,
  MetricsIcon,
  OverviewIcon,
  PipelinesIcon,
  SettingsIcon,
} from '../ui/icons/Icons'
import { Brand } from './Brand'
import { UserCard } from './UserCard'
import styles from './Sidebar.module.css'

const projectNav = [
  { to: '/p/$projectId', label: 'Overview', Icon: OverviewIcon, exact: true },
  { to: '/p/$projectId/logs', label: 'Logs', Icon: LogsIcon, exact: false },
  { to: '/p/$projectId/errors', label: 'Errors', Icon: ErrorsIcon, exact: false },
  { to: '/p/$projectId/metrics', label: 'Metrics', Icon: MetricsIcon, exact: false },
  { to: '/p/$projectId/pipelines', label: 'Pipelines', Icon: PipelinesIcon, exact: false },
  { to: '/p/$projectId/connect', label: 'Connect app', Icon: ConnectIcon, exact: false },
  { to: '/p/$projectId/settings', label: 'Settings', Icon: SettingsIcon, exact: false },
] as const

/**
 * Sticky sidebar: brand, project switcher, the project's pages and the user
 * card. Project links only show while a project is open.
 */
export function Sidebar() {
  const { projectId } = useParams({ strict: false })

  return (
    <nav aria-label="Main" className={styles.sidebar}>
      <div className={styles.brand}>
        <Brand />
      </div>

      <Link to="/projects" className={styles.switcher}>
        <span className={styles.switcherLabel}>{projectId ? 'Switch project' : 'All projects'}</span>
        <ChevronsUpDownIcon size={14} className={styles.switcherIcon} />
      </Link>

      {projectId && (
        <ul className={styles.links}>
          {projectNav.map(({ to, label, Icon, exact }) => (
            <li key={to}>
              <Link to={to} params={{ projectId }} activeOptions={{ exact }} className={styles.link}>
                <Icon />
                {label}
              </Link>
            </li>
          ))}
        </ul>
      )}

      <div className={styles.spacer} />
      <UserCard />
    </nav>
  )
}
