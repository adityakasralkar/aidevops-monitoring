import type { Ref } from 'react'
import { MenuIcon } from '../ui/icons/Icons'
import { Brand } from './Brand'
import { SIDEBAR_ID } from './Sidebar'
import styles from './MobileBar.module.css'

interface MobileBarProps {
  menuOpen: boolean
  onOpenMenu: () => void
  menuButtonRef?: Ref<HTMLButtonElement>
}

/** Phone only: wordmark and the button that opens the sidebar drawer. */
export function MobileBar({ menuOpen, onOpenMenu, menuButtonRef }: MobileBarProps) {
  return (
    <header className={styles.bar}>
      <Brand />
      <button
        ref={menuButtonRef}
        type="button"
        className={styles.menu}
        onClick={onOpenMenu}
        aria-label="Open menu"
        aria-controls={SIDEBAR_ID}
        aria-expanded={menuOpen}
      >
        <MenuIcon size={20} />
      </button>
    </header>
  )
}
