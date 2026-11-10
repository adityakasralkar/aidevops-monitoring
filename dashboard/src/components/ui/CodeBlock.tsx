import { CopyButton } from './CopyButton'
import styles from './CodeBlock.module.css'

interface CodeBlockProps {
  code: string
  /** Small caption in the header bar, e.g. "Same for every language". */
  caption?: string
  /** Show a Copy button; its label, e.g. "Copy all settings". */
  copyLabel?: string
  /** Wrap long lines (narrow columns) instead of scrolling sideways. */
  wrap?: boolean
}

/** Dark box for commands and settings; scrolls sideways instead of wrapping. */
export function CodeBlock({ code, caption, copyLabel, wrap = false }: CodeBlockProps) {
  const hasHeader = Boolean(caption || copyLabel)
  return (
    <div className={styles.block}>
      {hasHeader && (
        <div className={styles.header}>
          <span className={styles.caption}>{caption}</span>
          {copyLabel && (
            <CopyButton text={code} label={copyLabel} tone="dark">
              {copyLabel}
            </CopyButton>
          )}
        </div>
      )}
      <pre className={wrap ? `${styles.code} ${styles.wrap}` : styles.code} tabIndex={0}>
        {code}
      </pre>
    </div>
  )
}
