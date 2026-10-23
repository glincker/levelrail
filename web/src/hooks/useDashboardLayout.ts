import { useCallback, useState } from 'react'
import {
  DEFAULT_LAYOUT,
  moveWidget,
  normalizeLayout,
  toggleWidget,
  type DashboardLayout,
  type WidgetId,
} from '../lib/dashboardLayout'

const STORAGE_KEY = 'dashboard.layout.v1'

function load(): DashboardLayout {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    return normalizeLayout(raw ? JSON.parse(raw) : null)
  } catch {
    return DEFAULT_LAYOUT
  }
}

function save(layout: DashboardLayout) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(layout))
  } catch {
    // Storage can be blocked; the layout then lasts for this visit only.
  }
}

export function useDashboardLayout() {
  const [layout, setLayout] = useState(load)
  const update = useCallback((next: DashboardLayout) => {
    setLayout(next)
    save(next)
  }, [])
  return {
    layout,
    move: (id: WidgetId, to: number) => {
      update(moveWidget(layout, id, to))
    },
    toggle: (id: WidgetId) => {
      update(toggleWidget(layout, id))
    },
    reset: () => {
      update(DEFAULT_LAYOUT)
    },
  }
}
