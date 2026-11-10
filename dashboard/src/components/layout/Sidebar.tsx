import { Link, useParams } from '@tanstack/react-router'
import type { Ref } from 'react'
import {
  ChevronsUpDownIcon,
  CloseIcon,
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

export const SIDEBAR_ID = 'main-nav'

interface SidebarProps {
  /** Phone only: the drawer is showing. */
  open: boolean
  /** Phone only: close the drawer (after a link tap or the close button). */
  onClose: () => void
  closeButtonRef?: Ref<HTMLButtonElement>
}

/**
 * Brand, project switcher, the project's pages and the user card.
 * Desktop: sticky panel. Tablet: icon rail. Phone: slide-in drawer.
 */
export function Sidebar({ open, onClose, closeButtonRef }: SidebarProps) {
  const { projectId } = useParams({ strict: false })

  return (
    <nav id={SIDEBAR_ID} aria-label="Main" className={open ? `${styles.sidebar} ${styles.open}` : styles.sidebar}>
      <div className={styles.brandRow}>
        <Brand collapsible />
        <button ref={closeButtonRef} type="button" className={styles.close} onClick={onClose} aria-label="Close menu">
          <CloseIcon size={18} />
        </button>
      </div>

      <Link to="/projects" className={styles.switcher} title="Switch project" onClick={onClose}>
        <span className={styles.label}>{projectId ? 'Switch project' : 'All projects'}</span>
        <ChevronsUpDownIcon size={14} className={styles.switcherIcon} />
      </Link>

      {projectId && (
        <ul className={styles.links}>
          {projectNav.map(({ to, label, Icon, exact }) => (
            <li key={to}>
              <Link
                to={to}
                params={{ projectId }}
                activeOptions={{ exact }}
                className={styles.link}
                title={label}
                onClick={onClose}
              >
                <Icon />
                <span className={styles.label}>{label}</span>
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
