import { useEffect, useCallback } from 'react'

const RECENTLY_VIEWED_KEY = 'levelrail:recently-viewed'
const MAX_RECENT = 5

interface RecentPage {
  path: string
  title: string
  viewedAt: number
}

export function useRecentlyViewedPages(
  currentPath: string,
  currentTitle: string,
) {
  const getRecent = useCallback((): RecentPage[] => {
    try {
      const stored = localStorage.getItem(RECENTLY_VIEWED_KEY)
      return stored ? (JSON.parse(stored) as Array<RecentPage>) : []
    } catch {
      return []
    }
  }, [])

  useEffect(() => {
    if (!currentPath || currentPath === '/') return

    try {
      const recent = getRecent()
      const filtered = recent.filter((p) => p.path !== currentPath)
      const updated = [
        { path: currentPath, title: currentTitle, viewedAt: Date.now() },
        ...filtered,
      ].slice(0, MAX_RECENT)
      localStorage.setItem(RECENTLY_VIEWED_KEY, JSON.stringify(updated))
    } catch {
      // Silently fail if localStorage is unavailable
    }
  }, [currentPath, currentTitle, getRecent])

  return getRecent()
}
