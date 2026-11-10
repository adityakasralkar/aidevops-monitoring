import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { DiagnosisStatus, ErrorDetail, LogPage } from '../../lib/types'
import type { Range } from '../overview/range'

const PAGE_SIZE = 50

/** ERROR / CRITICAL logs, newest first; refreshes every 15 seconds. */
export function useErrors(
  projectId: string,
  filters: { range: Range; status?: DiagnosisStatus; service?: string },
) {
  return useInfiniteQuery({
    queryKey: ['errors', projectId, filters],
    queryFn: ({ pageParam, signal }) => {
      const params = new URLSearchParams({ range: filters.range, limit: String(PAGE_SIZE) })
      if (filters.status) params.set('status', filters.status)
      if (filters.service) params.set('service', filters.service)
      if (pageParam) params.set('cursor', pageParam)
      return api<LogPage>(`/projects/${encodeURIComponent(projectId)}/errors?${params}`, { signal })
    },
    initialPageParam: '',
    getNextPageParam: (last) => last.next_cursor || undefined,
    refetchInterval: 15_000,
    placeholderData: (previous) => previous,
  })
}

/** One error with the metrics and logs from the 10 minutes before it. */
export function useErrorDetail(projectId: string, errorId: string | undefined) {
  return useQuery({
    queryKey: ['error', projectId, errorId],
    queryFn: ({ signal }) =>
      api<ErrorDetail>(`/projects/${encodeURIComponent(projectId)}/errors/${encodeURIComponent(errorId ?? '')}`, {
        signal,
      }),
    enabled: Boolean(errorId),
    // A pending diagnosis can complete later; look again now and then.
    refetchInterval: (query) => (query.state.data?.diagnosis_status === 'PENDING' ? 30_000 : false),
  })
}
