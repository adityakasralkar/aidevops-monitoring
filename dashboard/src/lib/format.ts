// All display formatting goes through here so every screen says things the same way.

const MINUTE = 60
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

/** Seconds between two times, never negative (clocks can drift a little). */
export function secondsBetween(from: Date, to: Date): number {
  return Math.max(0, Math.round((to.getTime() - from.getTime()) / 1000))
}

/** A length of time in short words: "40s", "5 min", "2 h", "3 days". */
export function duration(seconds: number): string {
  if (seconds < MINUTE) return `${seconds}s`
  if (seconds < HOUR) return `${Math.floor(seconds / MINUTE)} min`
  if (seconds < DAY) return `${Math.floor(seconds / HOUR)} h`
  const days = Math.floor(seconds / DAY)
  return days === 1 ? '1 day' : `${days} days`
}

/** "4s ago", "5 min ago", "Never"; older than 30 days shows the date. */
export function timeAgo(value: string | null | undefined, now = new Date()): string {
  if (!value) return 'Never'
  const date = new Date(value)
  const seconds = secondsBetween(date, now)
  if (seconds < 5) return 'Just now'
  if (seconds > 30 * DAY) return dateTime(value)
  return `${duration(seconds)} ago`
}

/** "9 Nov 2026" */
export function dateOnly(value: string): string {
  return new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(value))
}

/** "9 Nov 2026, 14:02" */
export function dateTime(value: string): string {
  return new Intl.DateTimeFormat('en-GB', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

const integer = new Intl.NumberFormat('en-GB', { maximumFractionDigits: 0 })
const oneDecimal = new Intl.NumberFormat('en-GB', { maximumFractionDigits: 1 })

/** 1248 → "1,248"; 12500 → "12.5k"; 2300000 → "2.3M". */
export function compactNumber(value: number): string {
  const abs = Math.abs(value)
  if (abs >= 1_000_000) return `${oneDecimal.format(value / 1_000_000)}M`
  if (abs >= 10_000) return `${oneDecimal.format(value / 1_000)}k`
  if (abs >= 10) return integer.format(value)
  return oneDecimal.format(value)
}

/** Latency: 0.4 → "0.4 ms", 840 → "840 ms", 1500 → "1.5 s". */
export function ms(value: number | null | undefined): string {
  if (value === null || value === undefined) return '—'
  if (value >= 1000) return `${oneDecimal.format(value / 1000)} s`
  if (value >= 10) return `${integer.format(value)} ms`
  return `${oneDecimal.format(value)} ms`
}

/** 0.0214 → "2.1%"; null → "—". */
export function percent(fraction: number | null | undefined): string {
  if (fraction === null || fraction === undefined) return '—'
  const p = fraction * 100
  if (p > 0 && p < 0.1) return '<0.1%'
  return `${oneDecimal.format(p)}%`
}

/** Chart axis label: "14:05" within a day, "9 Nov" for longer ranges. */
export function axisTime(value: string, spansDays: boolean): string {
  const date = new Date(value)
  return spansDays
    ? new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short' }).format(date)
    : new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit' }).format(date)
}

/** "14:02:11" */
export function clockTime(value: string): string {
  return new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value))
}
