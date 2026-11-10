import type { ReactNode } from 'react'
import styles from './GlassCard.module.css'

interface GlassCardProps {
  children: ReactNode
  /** Inner spacing: `md` for most cards, `none` when the content sets its own (lists, tables). */
  padding?: 'none' | 'md'
  as?: 'div' | 'section' | 'article'
  className?: string
}

/** The frosted glass panel every screen is built from. */
export function GlassCard({ children, padding = 'md', as: Tag = 'div', className }: GlassCardProps) {
  const classes = [styles.card, padding === 'md' ? styles.padded : '', className ?? ''].join(' ').trim()
  return <Tag className={classes}>{children}</Tag>
}
