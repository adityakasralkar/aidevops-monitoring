/** Time ranges the server understands, shortest first. */
export const RANGES = ['15m', '1h', '6h', '24h', '7d'] as const
export type Range = (typeof RANGES)[number]
export const DEFAULT_RANGE: Range = '1h'

/** Reads ?range= from the URL; anything unknown falls back to 1h. */
export function parseRangeSearch(search: Record<string, unknown>): { range?: Range } {
  const value = search.range
  return { range: RANGES.includes(value as Range) ? (value as Range) : undefined }
}
