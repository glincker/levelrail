import { useLayoutEffect } from 'react'
import { MAIN_CONTENT_ID } from './SkipLink'

// The router has no scroll restoration and <main> persists across routes, so
// its offset from the previous page would carry over. A hash deep link keeps
// its own target scroll.
export function useResetMainScroll(pathname: string, hash: string): void {
  useLayoutEffect(() => {
    if (hash) return
    const el = document.getElementById(MAIN_CONTENT_ID)
    if (el && el.scrollTop !== 0) el.scrollTop = 0
  }, [pathname, hash])
}
