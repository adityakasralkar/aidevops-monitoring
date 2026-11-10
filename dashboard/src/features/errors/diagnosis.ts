import type { StatusTone } from '../../components/ui/StatusDot'
import type { DiagnosisStatus } from '../../lib/types'

/** How each diagnosis status is shown. */
export const diagnosisLabel: Record<DiagnosisStatus, { text: string; tone: StatusTone }> = {
  PENDING: { text: 'AI pending', tone: 'quiet' },
  COMPLETED: { text: 'Diagnosed', tone: 'ok' },
  FAILED: { text: 'Diagnosis failed', tone: 'error' },
}
