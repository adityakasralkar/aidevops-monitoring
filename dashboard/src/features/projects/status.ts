import type { StatusTone } from '../../components/ui/StatusDot'
import { duration, secondsBetween } from '../../lib/format'
import type { Project } from '../../lib/types'

/** Data counts as "receiving" if the key was used in the last 5 minutes. */
const RECEIVING_WINDOW_SECONDS = 5 * 60

/**
 * A project's status, from the only signal the list has: when its API key
 * last sent data.
 */
export function projectStatus(project: Project, now = new Date()): { tone: StatusTone; text: string } {
  if (!project.last_used_at) return { tone: 'idle', text: 'Waiting for first data' }
  const quietFor = secondsBetween(new Date(project.last_used_at), now)
  if (quietFor <= RECEIVING_WINDOW_SECONDS) return { tone: 'ok', text: 'Receiving data' }
  return { tone: 'quiet', text: `No data for ${duration(quietFor)}` }
}
