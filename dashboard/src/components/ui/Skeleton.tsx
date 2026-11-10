import styles from './Skeleton.module.css'

/** Grey placeholder bar shown while data loads. */
export function Skeleton({ width = '100%', height = 12 }: { width?: number | string; height?: number }) {
  return <span className={styles.bar} style={{ width, height }} aria-hidden="true" />
}
