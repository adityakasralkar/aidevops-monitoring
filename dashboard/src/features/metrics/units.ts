import { compactNumber, ms } from '../../lib/format'

/**
 * OpenTelemetry units use the UCUM style: "s", "ms", "By" (bytes), "1"
 * (a plain number) and "{name}" annotations such as "{conn}".
 */
export function unitLabel(unit: string | undefined): string {
  if (!unit || unit === '1') return ''
  if (unit === 'By') return 'bytes'
  if (unit.startsWith('{') && unit.endsWith('}')) return unit.slice(1, -1)
  return unit
}

const BYTE_STEPS = ['B', 'KB', 'MB', 'GB', 'TB']

/** A value in its own unit: durations as ms/s, bytes as KB/MB, others as plain numbers. */
export function formatValue(value: number | null | undefined, unit: string | undefined): string {
  if (value === null || value === undefined) return '—'
  if (unit === 's') return ms(value * 1000)
  if (unit === 'ms') return ms(value)
  if (unit === 'By') {
    let v = value
    let i = 0
    while (Math.abs(v) >= 1024 && i < BYTE_STEPS.length - 1) {
      v /= 1024
      i++
    }
    return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1)} ${BYTE_STEPS[i]}`
  }
  let label = unitLabel(unit)
  // "{connection}" style units are written in the singular: 4 connections, 1 connection.
  if (unit?.startsWith('{') && value !== 1 && label && !label.endsWith('s')) label = `${label}s`
  return label ? `${compactNumber(value)} ${label}` : compactNumber(value)
}

/** Bucket edge for the distribution chart: "≤ 25 ms", "> 1 s". */
export function boundLabel(bound: number, unit: string | undefined): string {
  return formatValue(bound, unit).replace(/\.0(?=\D)/, '')
}

export const typeText: Record<'gauge' | 'sum' | 'histogram', { short: string; about: string }> = {
  gauge: { short: 'gauge', about: 'A level at a moment, e.g. memory in use. The chart shows the latest value in each step.' },
  sum: {
    short: 'sum',
    about: 'A counter. For counts that only go up, the chart shows how much was added in each step; for counts that go up and down, the level.',
  },
  histogram: {
    short: 'histogram',
    about: 'A spread of values, e.g. how long requests took. The chart shows the p95 and p50 of each step.',
  },
}
