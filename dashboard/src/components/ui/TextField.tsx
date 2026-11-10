import { useId, type InputHTMLAttributes } from 'react'
import styles from './TextField.module.css'

interface TextFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string
  /** Help text under the field. */
  hint?: string
  /** Error under the field; also marks the field invalid. */
  error?: string
}

/** Label + input + hint or error, wired together for screen readers. */
export function TextField({ label, hint, error, id, ...rest }: TextFieldProps) {
  const autoId = useId()
  const inputId = id ?? autoId
  const noteId = `${inputId}-note`
  const note = error ?? hint

  return (
    <div className={styles.field}>
      <label htmlFor={inputId} className={styles.label}>
        {label}
      </label>
      <input
        {...rest}
        id={inputId}
        className={styles.input}
        aria-invalid={error ? true : undefined}
        aria-describedby={note ? noteId : undefined}
      />
      {note && (
        <span id={noteId} className={error ? styles.error : styles.hint}>
          {note}
        </span>
      )}
    </div>
  )
}
