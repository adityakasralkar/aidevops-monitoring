import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { PipelineRun } from '../../lib/types'
import type { Range } from '../overview/range'

export const RESULTS = ['all', 'passed', 'failed'] as const
export type ResultFilter = (typeof RESULTS)[number]

/** URL state (?run=…&result=failed&range=7d). */
export interface PipelineSearch {
  run?: string
  result?: Exclude<ResultFilter, 'all'>
  range?: Range
}

export function parsePipelineSearch(search: Record<string, unknown>): PipelineSearch {
  const run = typeof search.run === 'string' && search.run.trim() ? search.run.trim().slice(0, 100) : undefined
  const result = search.result === 'passed' || search.result === 'failed' ? search.result : undefined
  const ranges: Range[] = ['15m', '1h', '6h', '24h', '7d']
  return { run, result, range: ranges.includes(search.range as Range) ? (search.range as Range) : undefined }
}

/** CI runs reported by the project's pipelines; refreshes every 30 seconds. */
export function usePipelines(projectId: string, range: Range) {
  return useQuery({
    queryKey: ['pipelines', projectId, range],
    queryFn: ({ signal }) =>
      api<{ runs: PipelineRun[] }>(`/projects/${encodeURIComponent(projectId)}/pipelines?range=${range}`, { signal }).then(
        (r) => r.runs,
      ),
    refetchInterval: 30_000,
  })
}
