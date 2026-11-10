import styles from './LineChart.module.css'

export interface ChartSeries {
  label: string
  /** One value per time step; null leaves a gap. */
  values: (number | null)[]
  tone: 'main' | 'error' | 'muted'
  /** Fill the area under this line (lime). */
  area?: boolean
}

interface LineChartProps {
  series: ChartSeries[]
  /** Labels under the chart, spread evenly from first to last step. */
  xLabels: string[]
  /** Read out by screen readers: what the chart shows. */
  summary: string
  height?: number
}

const WIDTH = 600
const PAD_TOP = 8

/**
 * Simple SVG line chart. The drawing stretches to the card's width;
 * lines keep a constant thickness however wide it gets.
 */
export function LineChart({ series, xLabels, summary, height = 180 }: LineChartProps) {
  const steps = Math.max(...series.map((s) => s.values.length), 0)
  const max = Math.max(1, ...series.flatMap((s) => s.values.filter((v): v is number => v !== null)))
  const x = (i: number) => (steps <= 1 ? WIDTH / 2 : (i / (steps - 1)) * WIDTH)
  const y = (v: number) => height - 1 - (v / max) * (height - 1 - PAD_TOP)

  /** Unbroken runs of values, so gaps (null) stay gaps. */
  const runs = (values: (number | null)[]) => {
    const out: [number, number][][] = []
    let current: [number, number][] = []
    values.forEach((v, i) => {
      if (v === null) {
        if (current.length) out.push(current)
        current = []
      } else current.push([x(i), y(v)])
    })
    if (current.length) out.push(current)
    return out
  }

  return (
    <figure className={styles.figure}>
      <svg
        role="img"
        aria-label={summary}
        viewBox={`0 0 ${WIDTH} ${height}`}
        preserveAspectRatio="none"
        className={styles.svg}
        style={{ height }}
      >
        {[0.25, 0.5, 0.75].map((f) => (
          <line key={f} x1="0" x2={WIDTH} y1={height * f} y2={height * f} className={styles.grid} />
        ))}
        <line x1="0" x2={WIDTH} y1={height - 1} y2={height - 1} className={styles.base} />
        {series.map((s) =>
          runs(s.values).map((run, i) => {
            const points = run.map(([px, py]) => `${px},${py}`).join(' ')
            const single = run.length === 1
            return (
              <g key={`${s.label}-${i}`} className={styles[s.tone]}>
                {s.area && !single && (
                  <polygon
                    className={styles.area}
                    points={`${run[0][0]},${height} ${points} ${run[run.length - 1][0]},${height}`}
                  />
                )}
                {single ? (
                  <circle cx={run[0][0]} cy={run[0][1]} r="2.5" className={styles.dot} />
                ) : (
                  <polyline points={points} className={styles.line} vectorEffect="non-scaling-stroke" />
                )}
              </g>
            )
          }),
        )}
      </svg>
      <figcaption className={styles.axis} aria-hidden="true">
        {xLabels.map((label, i) => (
          <span key={i}>{label}</span>
        ))}
      </figcaption>
    </figure>
  )
}
