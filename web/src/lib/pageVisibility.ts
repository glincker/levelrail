import { useEffect, useState } from 'react'

export function isPageVisible(): boolean {
  return (
    typeof document === 'undefined' || document.visibilityState === 'visible'
  )
}

export function usePageVisible(): boolean {
  const [visible, setVisible] = useState(isPageVisible)
  useEffect(() => {
    const on = () => setVisible(isPageVisible())
    document.addEventListener('visibilitychange', on)
    return () => document.removeEventListener('visibilitychange', on)
  }, [])
  return visible
}

// Interval that skips ticks while the tab is hidden and ticks once on return.
export function setVisibleInterval(fn: () => void, ms: number): () => void {
  const id = window.setInterval(() => {
    if (isPageVisible()) fn()
  }, ms)
  const on = () => {
    if (isPageVisible()) fn()
  }
  document.addEventListener('visibilitychange', on)
  return () => {
    window.clearInterval(id)
    document.removeEventListener('visibilitychange', on)
  }
}
