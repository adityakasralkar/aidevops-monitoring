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
