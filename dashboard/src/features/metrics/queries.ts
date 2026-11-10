import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import type { MetricInfo, MetricSeries } from '../../lib/types'
import type { Range } from '../overview/range'

/** Every metric each service sent in the range, built-in and custom. */
export function useMetricCatalog(projectId: string, range: Range) {
  return useQuery({
    queryKey: ['metrics', projectId, range],
    queryFn: ({ signal }) =>
      api<{ metrics: MetricInfo[] }>(`/projects/${encodeURIComponent(projectId)}/metrics?range=${range}`, { signal }).then(
        (r) => r.metrics,
      ),
    refetchInterval: 30_000,
  })
}

/** One metric of one service as chart points (plus bucket counts for histograms). */
export function useMetricSeries(projectId: string, metric: { name: string; service: string } | undefined, range: Range) {
  return useQuery({
    queryKey: ['metric-series', projectId, metric?.name, metric?.service, range],
    queryFn: ({ signal }) => {
      const params = new URLSearchParams({ name: metric?.name ?? '', service: metric?.service ?? '', range })
      return api<{ series: MetricSeries; truncated: boolean }>(
        `/projects/${encodeURIComponent(projectId)}/metrics/series?${params}`,
        { signal },
      )
    },
    enabled: Boolean(metric),
    refetchInterval: 30_000,
    placeholderData: (previous) => previous,
  })
}
