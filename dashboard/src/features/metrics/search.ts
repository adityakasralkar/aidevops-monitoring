import { RANGES, type Range } from '../overview/range'

/** URL state of the Metrics page (?name=…&service=…&range=1h). */
export interface MetricSearch {
  name?: string
  service?: string
  range?: Range
}

export function parseMetricSearch(search: Record<string, unknown>): MetricSearch {
  const str = (v: unknown) => (typeof v === 'string' && v.trim() ? v.trim().slice(0, 200) : undefined)
  return {
    name: str(search.name),
    service: str(search.service),
    range: RANGES.includes(search.range as Range) ? (search.range as Range) : undefined,
  }
}
