import type { DashboardDoc } from '../types/dashboards'

export const START_TAB = 'start'
export const METRICS_TAB = 'metrics'
export const BUILTIN_TABS: readonly string[] = [START_TAB, METRICS_TAB]

// The bar shows this many tabs; the rest move into the overflow menu.
export const MAX_VISIBLE_TABS = 4

export interface TabPrefs {
  order: string[]
  hidden: string[]
}

export const DEFAULT_TAB_PREFS: TabPrefs = {
  order: [...BUILTIN_TABS],
  hidden: [],
}

export const savedTabId = (dashboardId: string) => `d:${dashboardId}`

export const savedDashboardId = (tabId: string): string | null =>
  tabId.startsWith('d:') ? tabId.slice(2) : null

// Fleet-wide widgets only: neither reads a target, so one doc fits any install.
export function metricsDoc(name: string): DashboardDoc {
  return {
    version: 1,
    name,
    time_range: '24h',
    widgets: [
      { id: 'resource-counts-1', type: 'resource_counts', size: 'L' },
      { id: 'cost-1', type: 'cost', size: 'L' },
    ],
  }
}

const strings = (v: unknown): string[] =>
  Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []

/** normalizeTabPrefs repairs stored data: drops junk and duplicates, and never hides Start. */
export function normalizeTabPrefs(raw: unknown): TabPrefs {
  const obj = (raw ?? {}) as { order?: unknown; hidden?: unknown }
  const order = [...new Set(strings(obj.order))]
  for (const id of BUILTIN_TABS) if (!order.includes(id)) order.push(id)
  const hidden = [...new Set(strings(obj.hidden))].filter(
    (id) => id !== START_TAB,
  )
  return { order, hidden }
}

/** resolveTabs lists every available tab id in user order; new dashboards are appended. */
export function resolveTabs(prefs: TabPrefs, dashboardIds: string[]): string[] {
  const available = new Set([...BUILTIN_TABS, ...dashboardIds.map(savedTabId)])
  const ordered = prefs.order.filter((id) => available.has(id))
  const seen = new Set(ordered)
  for (const id of available) if (!seen.has(id)) ordered.push(id)
  return ordered
}

export const shownTabs = (all: string[], prefs: TabPrefs): string[] =>
  all.filter((id) => !prefs.hidden.includes(id))

export const hiddenTabs = (all: string[], prefs: TabPrefs): string[] =>
  all.filter((id) => prefs.hidden.includes(id))

/** splitTabs keeps the selected tab on the bar by swapping it into the last slot. */
export function splitTabs(
  tabs: string[],
  selected: string,
  max = MAX_VISIBLE_TABS,
): { visible: string[]; overflow: string[] } {
  if (tabs.length <= max) return { visible: tabs, overflow: [] }
  const visible = tabs.slice(0, max)
  const overflow = tabs.slice(max)
  if (overflow.includes(selected)) {
    const swapped = visible[max - 1] as string
    visible[max - 1] = selected
    overflow[overflow.indexOf(selected)] = swapped
  }
  return { visible, overflow }
}

export function moveTab(prefs: TabPrefs, id: string, to: number): TabPrefs {
  const from = prefs.order.indexOf(id)
  if (from < 0) return prefs
  const target = Math.max(0, Math.min(prefs.order.length - 1, to))
  if (target === from) return prefs
  const order = [...prefs.order]
  order.splice(from, 1)
  order.splice(target, 0, id)
  return { ...prefs, order }
}

export function hideTab(prefs: TabPrefs, id: string): TabPrefs {
  if (id === START_TAB || prefs.hidden.includes(id)) return prefs
  return { ...prefs, hidden: [...prefs.hidden, id] }
}

export function showTab(prefs: TabPrefs, id: string): TabPrefs {
  if (!prefs.hidden.includes(id)) return prefs
  return { ...prefs, hidden: prefs.hidden.filter((h) => h !== id) }
}

/** defaultTab lands on a board once there are apps, and on Start before the first one. */
export function defaultTab(hasApps: boolean, shown: string[]): string {
  if (!hasApps) return START_TAB
  const saved = shown.find((id) => savedDashboardId(id) !== null)
  if (saved) return saved
  return shown.includes(METRICS_TAB) ? METRICS_TAB : START_TAB
}

/** pickSelected keeps the stored tab while it is still shown, else uses the fallback. */
export function pickSelected(
  stored: string | null,
  shown: string[],
  fallback: string = START_TAB,
): string {
  return stored !== null && shown.includes(stored) ? stored : fallback
}

/** boardTab maps a ?board= value (a saved id or a built-in tab) to a tab id. */
export function boardTab(board: string): string {
  return BUILTIN_TABS.includes(board) ? board : savedTabId(board)
}
