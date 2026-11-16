import { useEffect, useState } from 'react'
import { MAIN_CONTENT_ID } from './SkipLink'

export const MAIN_SENTINEL_ATTR = 'data-main-sentinel'

// A sentinel at the top of the scroller leaves the viewport once the page
// scrolls, so no scroll listener runs on the main thread.
export function useMainScrolled(): boolean {
  const [scrolled, setScrolled] = useState(false)
  useEffect(() => {
    const el = document.getElementById(MAIN_CONTENT_ID)
    const sentinel = el?.querySelector(`[${MAIN_SENTINEL_ATTR}]`)
    if (!el || !sentinel || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver(
      (entries) => {
        const last = entries[entries.length - 1]
        if (last) setScrolled(!last.isIntersecting)
      },
      { root: el, threshold: 0 },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [])
  return scrolled
}
