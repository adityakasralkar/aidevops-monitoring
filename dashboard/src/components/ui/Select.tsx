import { useId, type SelectHTMLAttributes } from 'react'
import styles from './Select.module.css'

interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  /** Read by screen readers; shown only when `showLabel` is set. */
  label: string
  showLabel?: boolean
  options: { value: string; label: string }[]
}

/** The browser's own dropdown, styled to match the inputs. */
export function Select({ label, showLabel = false, options, id, className, ...rest }: SelectProps) {
  const autoId = useId()
  const selectId = id ?? autoId
  return (
    <span className={[styles.wrap, className ?? ''].join(' ').trim()}>
      <label htmlFor={selectId} className={showLabel ? styles.label : 'sr-only'}>
        {label}
      </label>
      <select {...rest} id={selectId} className={styles.select}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </span>
  )
}
