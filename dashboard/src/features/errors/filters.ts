import { RANGES, type Range } from '../overview/range'
import type { DiagnosisStatus } from '../../lib/types'

export const STATUSES = ['ALL', 'PENDING', 'COMPLETED', 'FAILED'] as const
export type StatusFilter = (typeof STATUSES)[number]

/** URL state of the Errors page (?id=…&status=PENDING&service=…&range=24h). */
export interface ErrorSearch {
  /** The selected error. */
  id?: string
  status?: DiagnosisStatus
  service?: string
  range?: Range
}

export const DEFAULT_ERROR_RANGE: Range = '24h'

export function parseErrorSearch(search: Record<string, unknown>): ErrorSearch {
  const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim().slice(0, 200) : undefined)
  const status = typeof search.status === 'string' ? search.status.toUpperCase() : undefined
  const id = str(search.id)
  return {
    id: id && /^[0-9a-f]{24}$/i.test(id) ? id : undefined,
    status: STATUSES.includes(status as StatusFilter) && status !== 'ALL' ? (status as DiagnosisStatus) : undefined,
    service: str(search.service),
    range: RANGES.includes(search.range as Range) ? (search.range as Range) : undefined,
  }
}
