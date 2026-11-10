import styles from './HistogramBars.module.css'

interface HistogramBarsProps {
  /** One label per bar, e.g. "≤ 25 ms". */
  labels: string[]
  counts: number[]
  summary: string
}

/** Vertical bars for a histogram's buckets, tallest = 100%. */
export function HistogramBars({ labels, counts, summary }: HistogramBarsProps) {
  const max = Math.max(1, ...counts)
  return (
    <figure className={styles.figure} role="img" aria-label={summary}>
      <div className={styles.bars} aria-hidden="true">
        {counts.map((count, i) => (
          <div key={i} className={styles.column}>
            <span className={styles.count}>{count > 0 ? count.toLocaleString('en-GB') : ''}</span>
            <div className={styles.bar} style={{ height: `${Math.max(count > 0 ? 2 : 0, (count / max) * 100)}%` }} />
          </div>
        ))}
      </div>
      <div className={styles.labels} aria-hidden="true">
        {labels.map((label, i) => (
          <span key={i}>{label}</span>
        ))}
      </div>
    </figure>
  )
}
