import { Outlet } from '@tanstack/react-router'
import { useCallback, useEffect, useRef, useState } from 'react'
import { MobileBar } from './MobileBar'
import { Sidebar } from './Sidebar'
import styles from './AppShell.module.css'

/**
 * Layout for signed-in pages: sidebar on the left, page on the right.
 * On phones the sidebar is a drawer opened from the top bar.
 */
export function AppShell() {
  const [menuOpen, setMenuOpen] = useState(false)
  const menuButtonRef = useRef<HTMLButtonElement>(null)
  const closeButtonRef = useRef<HTMLButtonElement>(null)
  const wasOpen = useRef(false)

  const closeMenu = useCallback(() => setMenuOpen(false), [])

  // While the drawer is open: Esc closes it and the page behind doesn't scroll.
  useEffect(() => {
    if (!menuOpen) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closeMenu()
    }
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    document.addEventListener('keydown', onKey)
    return () => {
      document.body.style.overflow = overflow
      document.removeEventListener('keydown', onKey)
    }
  }, [menuOpen, closeMenu])

  // Focus goes into the drawer when it opens and back to the menu button when it closes.
  useEffect(() => {
    if (menuOpen) closeButtonRef.current?.focus()
    else if (wasOpen.current) menuButtonRef.current?.focus()
    wasOpen.current = menuOpen
  }, [menuOpen])

  return (
    <div className={styles.shell}>
      <MobileBar menuOpen={menuOpen} onOpenMenu={() => setMenuOpen(true)} menuButtonRef={menuButtonRef} />
      {menuOpen && <div className={styles.backdrop} aria-hidden="true" onClick={closeMenu} />}
      <Sidebar open={menuOpen} onClose={closeMenu} closeButtonRef={closeButtonRef} />
      <main className={styles.main}>
        <Outlet />
      </main>
    </div>
  )
}
