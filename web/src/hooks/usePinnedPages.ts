import { useCallback } from 'react'

const PINNED_PAGES_KEY = 'levelrail:pinned-pages'

interface PinnedPage {
  path: string
  title: string
}

export function usePinnedPages() {
  const getPinned = useCallback((): PinnedPage[] => {
    try {
      const stored = localStorage.getItem(PINNED_PAGES_KEY)
      return stored ? (JSON.parse(stored) as Array<PinnedPage>) : []
    } catch {
      return []
    }
  }, [])

  const isPinned = useCallback(
    (path: string): boolean => {
      return getPinned().some((p) => p.path === path)
    },
    [getPinned],
  )

  const pin = useCallback(
    (path: string, title: string) => {
      try {
        const pinned = getPinned()
        if (!pinned.some((p) => p.path === path)) {
          pinned.push({ path, title })
          localStorage.setItem(PINNED_PAGES_KEY, JSON.stringify(pinned))
        }
      } catch {
        // Silently fail if localStorage is unavailable
      }
    },
    [getPinned],
  )

  const unpin = useCallback(
    (path: string) => {
      try {
        const pinned = getPinned().filter((p) => p.path !== path)
        localStorage.setItem(PINNED_PAGES_KEY, JSON.stringify(pinned))
      } catch {
        // Silently fail if localStorage is unavailable
      }
    },
    [getPinned],
  )

  const toggle = useCallback(
    (path: string, title: string) => {
      if (isPinned(path)) {
        unpin(path)
      } else {
        pin(path, title)
      }
    },
    [isPinned, pin, unpin],
  )

  return {
    pinned: getPinned(),
    isPinned,
    pin,
    unpin,
    toggle,
  }
}
