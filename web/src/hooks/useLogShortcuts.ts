import { useEffect } from 'react'

export interface LogShortcuts {
  focusSearch: () => void
  toggleWrap: () => void
  next: () => void
  previous: () => void
  close: () => void
}

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false
  }
  return (
    target.isContentEditable ||
    ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
  )
}

// Keyboard shortcuts for the log viewer: "/" focuses the search box, "w"
// toggles wrapping, "j" and "k" move the selection, Escape closes the detail.
// Ignored while typing in a field or when a modifier key is held.
export function useLogShortcuts(handlers: LogShortcuts): void {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) {
        return
      }
      if (e.key === 'Escape') {
        handlers.close()
        return
      }
      if (isTyping(e.target)) {
        return
      }
      switch (e.key) {
        case '/':
          e.preventDefault()
          handlers.focusSearch()
          break
        case 'w':
          handlers.toggleWrap()
          break
        case 'j':
          handlers.next()
          break
        case 'k':
          handlers.previous()
          break
        default:
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [handlers])
}
