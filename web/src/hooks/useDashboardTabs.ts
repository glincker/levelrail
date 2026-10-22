import { useCallback, useMemo, useState } from 'react'
import {
  DEFAULT_TAB_PREFS,
  START_TAB,
  hiddenTabs,
  hideTab,
  moveTab,
  normalizeTabPrefs,
  pickSelected,
  resolveTabs,
  shownTabs,
  showTab,
  type TabPrefs,
} from '../lib/dashboardTabs'

const TAB_KEY = 'dashboard.tab.v1'
const PREFS_KEY = 'dashboard.tabs.v1'

function loadPrefs(): TabPrefs {
  try {
    const raw = window.localStorage.getItem(PREFS_KEY)
    return normalizeTabPrefs(raw ? JSON.parse(raw) : null)
  } catch {
    return DEFAULT_TAB_PREFS
  }
}

function loadTab(): string | null {
  try {
    return window.localStorage.getItem(TAB_KEY)
  } catch {
    return null
  }
}

function store(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // Storage can be blocked; the choice then lasts for this visit only.
  }
}

/** Tab order, hidden tabs and the selected tab, kept in localStorage. */
export function useDashboardTabs(dashboardIds: string[]) {
  const [prefs, setPrefs] = useState(loadPrefs)
  // Held raw so a saved dashboard tab survives the moment before the list loads.
  const [stored, setStored] = useState(loadTab)
  const idsKey = dashboardIds.join('\n')
  const all = useMemo(
    () => resolveTabs(prefs, idsKey ? idsKey.split('\n') : []),
    [prefs, idsKey],
  )
  const shown = useMemo(() => shownTabs(all, prefs), [all, prefs])
  const hidden = useMemo(() => hiddenTabs(all, prefs), [all, prefs])
  const selected = pickSelected(stored, shown)

  const select = useCallback((id: string) => {
    setStored(id)
    store(TAB_KEY, id)
  }, [])
  const updatePrefs = useCallback((next: TabPrefs) => {
    setPrefs(next)
    store(PREFS_KEY, JSON.stringify(next))
  }, [])

  return {
    shown,
    hidden,
    selected,
    storedTab: stored ?? START_TAB,
    select,
    hide: (id: string) => {
      updatePrefs(hideTab(prefs, id))
      if (id === selected) select(START_TAB)
    },
    show: (id: string) => {
      updatePrefs(showTab(prefs, id))
    },
    move: (id: string, to: number) => {
      updatePrefs(moveTab(prefs, id, to))
    },
  }
}
