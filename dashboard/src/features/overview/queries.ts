import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { LogPage, Overview } from '../../lib/types'
import type { Range } from './range'

/** Health of a project for a time range; refreshes every 15 seconds. */
export function useOverview(projectId: string, range: Range) {
  return useQuery({
    queryKey: ['overview', projectId, range],
    queryFn: ({ signal }) =>
      api<Overview>(`/projects/${encodeURIComponent(projectId)}/overview?range=${range}`, { signal }),
    refetchInterval: 15_000,
    placeholderData: (previous) => previous, // keep the old numbers while a new range loads
  })
}

/** The newest ERROR / CRITICAL logs in the range. */
export function useLatestErrors(projectId: string, range: Range, limit = 3) {
  return useQuery({
    queryKey: ['errors', projectId, range, { limit }],
    queryFn: ({ signal }) =>
      api<LogPage>(`/projects/${encodeURIComponent(projectId)}/errors?range=${range}&limit=${limit}`, { signal }).then(
        (r) => r.items,
      ),
    refetchInterval: 15_000,
  })
}
