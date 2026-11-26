// Editor tab state for the SQL console. Pure reducer plus guarded
// localStorage persistence so a reload restores the open tabs.

export interface ConsoleTab {
  id: string
  // Custom title (saved query name); null shows the numbered default.
  title: string | null
  sql: string
  // Text last saved or loaded from a saved query or history entry.
  savedSql: string
}

export interface TabsState {
  tabs: ConsoleTab[]
  activeId: string
  // Monotonic counter so ids and default numbers never repeat.
  seq: number
}

export type TabsAction =
  | { type: 'open'; sql?: string; title?: string | null }
  | { type: 'close'; id: string }
  | { type: 'select'; id: string }
  | { type: 'edit'; id: string; sql: string }
  | { type: 'saved'; id: string; title: string; sql: string }

export function initialTabs(): TabsState {
  return {
    tabs: [{ id: 't1', title: null, sql: '', savedSql: '' }],
    activeId: 't1',
    seq: 1,
  }
}

export function activeTabOf(state: TabsState): ConsoleTab {
  return (
    state.tabs.find((t) => t.id === state.activeId) ??
    state.tabs[0] ?? { id: 't0', title: null, sql: '', savedSql: '' }
  )
}

export function isDirty(tab: ConsoleTab): boolean {
  return tab.sql !== tab.savedSql
}

// Position-based number for the default title ("Query 2").
export function tabNumber(state: TabsState, id: string): number {
  const n = Number(id.slice(1))
  return Number.isFinite(n) && n > 0
    ? n
    : state.tabs.findIndex((t) => t.id === id) + 1
}

export function tabsReducer(state: TabsState, action: TabsAction): TabsState {
  switch (action.type) {
    case 'open': {
      const sql = action.sql ?? ''
      const title = action.title ?? null
      const active = state.tabs.find((t) => t.id === state.activeId)
      // An untouched empty tab is reused instead of piling up blanks.
      if (active && active.sql === '' && active.title === null && sql !== '') {
        return {
          ...state,
          tabs: state.tabs.map((t) =>
            t.id === active.id ? { ...t, title, sql, savedSql: sql } : t,
          ),
        }
      }
      const seq = state.seq + 1
      const tab: ConsoleTab = {
        id: `t${String(seq)}`,
        title,
        sql,
        savedSql: sql,
      }
      return { tabs: [...state.tabs, tab], activeId: tab.id, seq }
    }
    case 'close': {
      const idx = state.tabs.findIndex((t) => t.id === action.id)
      if (idx < 0) {
        return state
      }
      const rest = state.tabs.filter((t) => t.id !== action.id)
      if (rest.length === 0) {
        const seq = state.seq + 1
        const id = `t${String(seq)}`
        return {
          tabs: [{ id, title: null, sql: '', savedSql: '' }],
          activeId: id,
          seq,
        }
      }
      const activeId =
        state.activeId === action.id
          ? (rest[Math.min(idx, rest.length - 1)] as ConsoleTab).id
          : state.activeId
      return { ...state, tabs: rest, activeId }
    }
    case 'select':
      return state.tabs.some((t) => t.id === action.id)
        ? { ...state, activeId: action.id }
        : state
    case 'edit':
      return {
        ...state,
        tabs: state.tabs.map((t) =>
          t.id === action.id ? { ...t, sql: action.sql } : t,
        ),
      }
    case 'saved':
      return {
        ...state,
        tabs: state.tabs.map((t) =>
          t.id === action.id
            ? { ...t, title: action.title, savedSql: action.sql }
            : t,
        ),
      }
  }
}

const storageKey = (database: string) => `lr.sqlConsole.tabs.${database}`

function isTab(v: unknown): v is ConsoleTab {
  if (typeof v !== 'object' || v === null) {
    return false
  }
  const o = v as Record<string, unknown>
  return (
    typeof o.id === 'string' &&
    /^t\d+$/.test(o.id) &&
    (o.title === null || typeof o.title === 'string') &&
    typeof o.sql === 'string' &&
    typeof o.savedSql === 'string'
  )
}

// Validates untrusted stored JSON; anything off falls back to a fresh state.
export function parseStoredTabs(raw: string | null): TabsState | null {
  if (!raw) {
    return null
  }
  try {
    const v: unknown = JSON.parse(raw)
    if (typeof v !== 'object' || v === null) {
      return null
    }
    const o = v as Record<string, unknown>
    if (!Array.isArray(o.tabs) || o.tabs.length === 0) {
      return null
    }
    const tabs = o.tabs.filter(isTab)
    if (tabs.length === 0) {
      return null
    }
    const maxId = Math.max(...tabs.map((t) => Number(t.id.slice(1))))
    const activeId =
      typeof o.activeId === 'string' && tabs.some((t) => t.id === o.activeId)
        ? o.activeId
        : (tabs[0] as ConsoleTab).id
    return { tabs, activeId, seq: maxId }
  } catch {
    return null
  }
}

export function loadTabs(database: string): TabsState {
  try {
    return (
      parseStoredTabs(window.localStorage.getItem(storageKey(database))) ??
      initialTabs()
    )
  } catch {
    return initialTabs()
  }
}

export function saveTabs(database: string, state: TabsState): void {
  try {
    window.localStorage.setItem(
      storageKey(database),
      JSON.stringify({ tabs: state.tabs, activeId: state.activeId }),
    )
  } catch {
    // Private mode or quota: tabs just do not persist.
  }
}
