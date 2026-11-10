import { useEffect, useId, useRef, type ReactNode } from 'react'
import { CloseIcon } from './icons/Icons'
import styles from './Modal.module.css'

interface ModalProps {
  title: string
  /** Called when the user closes it (✕, Esc or a click outside). */
  onClose: () => void
  /**
   * false: the popup can only be left through its own buttons (no ✕, Esc
   * or outside click). Used when closing by accident would lose something,
   * like an API key that is shown once.
   */
  dismissible?: boolean
  size?: 'md' | 'lg'
  children: ReactNode
}

/**
 * A popup built on the browser's <dialog> element: it keeps keyboard focus
 * inside, dims and blocks the page behind, and closes on Esc.
 * Render it only while it should be open. Mark the field that should get
 * focus first with `data-autofocus`.
 */
export function Modal({ title, onClose, dismissible = true, size = 'md', children }: ModalProps) {
  const ref = useRef<HTMLDialogElement>(null)
  const titleId = useId()

  useEffect(() => {
    const dialog = ref.current
    if (!dialog) return
    dialog.showModal()
    // showModal() focuses the first button (the ✕); start on the main field instead.
    dialog.querySelector<HTMLElement>('[data-autofocus]')?.focus()
    return () => dialog.close()
  }, [])

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      className={size === 'lg' ? `${styles.dialog} ${styles.lg}` : styles.dialog}
      onCancel={(event) => {
        // Esc: the browser would close the dialog itself; let React decide.
        event.preventDefault()
        if (dismissible) onClose()
      }}
      onClick={(event) => {
        // A click on the dialog element itself (not its content) is on the backdrop.
        if (dismissible && event.target === event.currentTarget) onClose()
      }}
    >
      <div className={styles.panel}>
        <div className={styles.header}>
          <h2 id={titleId} className={styles.title}>
            {title}
          </h2>
          {dismissible && (
            <button type="button" className={styles.close} onClick={onClose} aria-label="Close">
              <CloseIcon size={18} />
            </button>
          )}
        </div>
        {children}
      </div>
    </dialog>
  )
}
