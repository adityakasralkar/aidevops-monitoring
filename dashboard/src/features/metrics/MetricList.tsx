import { Link } from '@tanstack/react-router'
import type { MetricInfo } from '../../lib/types'
import type { MetricSearch } from './search'
import { typeText, unitLabel } from './units'
import styles from './Metrics.module.css'

interface MetricListProps {
  projectId: string
  metrics: MetricInfo[]
  selected: MetricInfo | undefined
  search: MetricSearch
}

/** Every metric; picking one opens its chart (kept in the URL). */
export function MetricList({ projectId, metrics, selected, search }: MetricListProps) {
  return (
    <ul className={styles.list} aria-label="Metrics">
      {metrics.map((m) => {
        const isSelected = m.name === selected?.name && m.service === selected?.service
        const unit = unitLabel(m.unit)
        return (
          <li key={`${m.service}/${m.name}`}>
            <Link
              to="/p/$projectId/metrics"
              params={{ projectId }}
              search={{ ...search, name: m.name, service: m.service }}
              replace
              className={isSelected ? `${styles.item} ${styles.selected}` : styles.item}
              aria-current={isSelected ? 'true' : undefined}
            >
              <span className={styles.itemText}>
                <span className={styles.itemName}>{m.name}</span>
                <span className={styles.itemMeta}>
                  {m.service}
                  {unit && ` · ${unit}`}
                </span>
              </span>
              <span className={styles.typeTag}>{typeText[m.type]?.short ?? m.type}</span>
            </Link>
          </li>
        )
      })}
    </ul>
  )
}
