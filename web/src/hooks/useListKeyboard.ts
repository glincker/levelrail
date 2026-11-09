import { useEffect, useState } from 'react'
import { getAppearance } from '../lib/appearance'

const ROW_SELECTOR =
  'tbody > tr, [role="rowgroup"] > [role="row"], [data-list-row]'
const FILTER_SELECTOR =
  '[data-list-filter] input, input[type="search"], input[role="searchbox"]'

function isTyping(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable ||
      ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
  )
}

function reducedMotion(): boolean {
  return (
    typeof globalThis.matchMedia === 'function' &&
    globalThis.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

/** useListKeyboard wires j/k, x, Enter, / and ? onto the rows inside the element given to setContainer. */
export function useListKeyboard<T extends HTMLElement>(): {
  setContainer: (el: T | null) => void
  hintOpen: boolean
  closeHint: () => void
} {
  const [container, setContainer] = useState<T | null>(null)
  const [hintOpen, setHintOpen] = useState(false)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const root = container
      if (
        !root ||
        getAppearance().listKeys === 'off' ||
        e.metaKey ||
        e.ctrlKey ||
        e.altKey ||
        isTyping(e.target)
      ) {
        return
      }
      // An open dialog or menu owns the keyboard unless focus is already in the list.
      if (
        !root.contains(document.activeElement) &&
        document.querySelector('[role="dialog"], [role="menu"]')
      ) {
        return
      }
      const rows = Array.from(root.querySelectorAll<HTMLElement>(ROW_SELECTOR))
      const current = rows.findIndex((r) => r.contains(document.activeElement))
      const move = (to: number) => {
        const row = rows[Math.max(0, Math.min(rows.length - 1, to))]
        if (!row) return
        row.tabIndex = -1
        row.focus({ preventScroll: true })
        row.scrollIntoView({
          block: 'nearest',
          behavior: reducedMotion() ? 'auto' : 'smooth',
        })
      }
      const row = current >= 0 ? rows[current] : undefined
      if (e.key === 'j' || e.key === 'k') {
        e.preventDefault()
        move(current < 0 ? 0 : current + (e.key === 'j' ? 1 : -1))
      } else if (e.key === 'x' && row) {
        e.preventDefault()
        row
          .querySelector<HTMLElement>(
            'input[type="checkbox"], [role="checkbox"]',
          )
          ?.click()
      } else if (e.key === 'Enter' && row && document.activeElement === row) {
        e.preventDefault()
        row.querySelector<HTMLElement>('a[href]')?.click()
      } else if (e.key === '/') {
        const input = document.querySelector<HTMLElement>(FILTER_SELECTOR)
        if (input) {
          e.preventDefault()
          input.focus()
        }
      } else if (e.key === '?') {
        e.preventDefault()
        setHintOpen((open) => !open)
      } else if (e.key === 'Escape') {
        setHintOpen(false)
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
    }
  }, [container])

  return {
    setContainer,
    hintOpen,
    closeHint: () => {
      setHintOpen(false)
    },
  }
}
