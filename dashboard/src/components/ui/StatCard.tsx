import type { ReactNode } from 'react'
import styles from './StatCard.module.css'

interface StatCardProps {
  label: string
  value: ReactNode
  unit?: string
  /** Small line under the value. */
  note?: ReactNode
  tone?: 'default' | 'error' | 'empty'
}

/** One headline number (KPI) with a label and a short note. */
export function StatCard({ label, value, unit, note, tone = 'default' }: StatCardProps) {
  return (
    <div className={styles.card}>
      <span className={styles.label}>{label}</span>
      <span className={`${styles.value} ${styles[tone]}`}>
        {value}
        {unit && <span className={styles.unit}> {unit}</span>}
      </span>
      {note && <span className={tone === 'error' ? `${styles.note} ${styles.noteError}` : styles.note}>{note}</span>}
    </div>
  )
}
