import { useEffect, useRef, useState } from 'react'
import styles from './CopyButton.module.css'

interface CopyButtonProps {
  text: string
  /** Screen-reader name, e.g. "Copy API key". */
  label?: string
}

/**
 * Puts text on the clipboard. The modern Clipboard API only works on https
 * or localhost; opened over plain http (e.g. a phone on the LAN) it is
 * missing, so fall back to the older copy command.
 */
async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    const area = document.createElement('textarea')
    area.value = text
    area.setAttribute('readonly', '')
    area.style.position = 'fixed'
    area.style.opacity = '0'
    // Inside a <dialog>, only elements in the dialog can take focus.
    const host = document.querySelector('dialog[open]') ?? document.body
    host.appendChild(area)
    area.select()
    try {
      return document.execCommand('copy')
    } catch {
      return false
    } finally {
      area.remove()
    }
  }
}

/** Copies text to the clipboard; shows "Copied" for 2 seconds. */
export function CopyButton({ text, label = 'Copy' }: CopyButtonProps) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const timer = useRef<number | undefined>(undefined)

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const copy = async () => {
    setState((await copyText(text)) ? 'copied' : 'failed')
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setState('idle'), 2000)
  }

  return (
    <button
      type="button"
      className={state === 'copied' ? `${styles.button} ${styles.copied}` : styles.button}
      onClick={copy}
      aria-label={label}
    >
      <span aria-live="polite">{state === 'copied' ? 'Copied' : state === 'failed' ? 'Select and copy' : 'Copy'}</span>
    </button>
  )
}
