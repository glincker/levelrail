import { useCallback, useMemo, useSyncExternalStore } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useRouterState } from '@tanstack/react-router'
import {
  groupForSlug,
  groupTabs,
  hiddenTabCount,
  type AppTabGroupId,
  type GroupedTabs,
} from '@/lib/appTabGroups'
import {
  appSlugFromPath,
  visibleAppSections,
  type AppNavItem,
} from '../shell/navModel'
import { staticSitesQueryOptions } from '../../queries/staticSites'

const SHOW_ALL_KEY = 'app.tabs.showAll'
const listeners = new Set<() => void>()

function readShowAll(): boolean {
  try {
    return window.localStorage.getItem(SHOW_ALL_KEY) === '1'
  } catch {
    return false
  }
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

function writeShowAll(value: boolean): void {
  try {
    window.localStorage.setItem(SHOW_ALL_KEY, value ? '1' : '0')
  } catch {
    // storage unavailable: the escape hatch lasts until reload
  }
  listeners.forEach((fn) => {
    fn()
  })
}

export interface AppTabsState {
  groups: GroupedTabs<AppNavItem>[]
  activeGroup: AppTabGroupId | undefined
  activeSlug: string | undefined
  isStatic: boolean
  hiddenCount: number
  showAll: boolean
  setShowAll: (value: boolean) => void
}

/** Grouped tabs for one app, trimmed to its type unless "Show all tabs" is on. */
export function useAppTabs(name: string): AppTabsState {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { data: staticSites } = useQuery(staticSitesQueryOptions())
  const isStatic = staticSites?.some((s) => s.name === name) ?? false
  const showAll = useSyncExternalStore(subscribe, readShowAll, () => false)
  const setShowAll = useCallback((value: boolean) => {
    writeShowAll(value)
  }, [])
  const activeSlug = appSlugFromPath(pathname)

  return useMemo(() => {
    const items = visibleAppSections(isStatic).flatMap((s) => s.items)
    return {
      groups: groupTabs(items, { isStatic, showAll }),
      activeGroup: activeSlug ? groupForSlug(activeSlug) : undefined,
      activeSlug,
      isStatic,
      hiddenCount: showAll ? 0 : hiddenTabCount(items, isStatic),
      showAll,
      setShowAll,
    }
  }, [isStatic, showAll, activeSlug, setShowAll])
}
