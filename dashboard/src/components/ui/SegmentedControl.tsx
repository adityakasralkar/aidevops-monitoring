import styles from './SegmentedControl.module.css'

interface SegmentedControlProps<T extends string> {
  label: string
  options: readonly T[]
  value: T
  onChange: (value: T) => void
}

/** A row of toggle buttons where exactly one is on (e.g. 15m · 1h · 24h). */
export function SegmentedControl<T extends string>({ label, options, value, onChange }: SegmentedControlProps<T>) {
  return (
    <div role="group" aria-label={label} className={styles.group}>
      {options.map((option) => (
        <button
          key={option}
          type="button"
          aria-pressed={option === value}
          className={styles.option}
          onClick={() => onChange(option)}
        >
          {option}
        </button>
      ))}
    </div>
  )
}
