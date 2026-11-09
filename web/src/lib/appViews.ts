// Legacy browser-only Apps views. Saved views now live on the server; this
// copy is read once by the migration in migrateLocalAppViews and then cleared.

export interface AppListFilters {
  tags: string[]
  environments: string[]
  query: string
}

export interface SavedAppView {
  name: string
  filters: AppListFilters
}

export const EMPTY_FILTERS: AppListFilters = {
  tags: [],
  environments: [],
  query: '',
}

const STORAGE_KEY = 'apps.savedViews.v1'

function isStringArray(v: unknown): v is string[] {
  return Array.isArray(v) && v.every((x) => typeof x === 'string')
}

function parseViews(raw: string | null): SavedAppView[] {
  if (!raw) {
    return []
  }
  try {
    const data: unknown = JSON.parse(raw)
    if (!Array.isArray(data)) {
      return []
    }
    const out: SavedAppView[] = []
    for (const item of data) {
      if (typeof item !== 'object' || item === null) {
        continue
      }
      const rec = item as Record<string, unknown>
      const f = rec.filters as Record<string, unknown> | undefined
      if (
        typeof rec.name === 'string' &&
        f &&
        isStringArray(f.tags) &&
        isStringArray(f.environments) &&
        typeof f.query === 'string'
      ) {
        out.push({
          name: rec.name,
          filters: {
            tags: f.tags,
            environments: f.environments,
            query: f.query,
          },
        })
      }
    }
    return out
  } catch {
    return []
  }
}

export function loadSavedViews(): SavedAppView[] {
  try {
    return parseViews(window.localStorage.getItem(STORAGE_KEY))
  } catch {
    return []
  }
}

export function storeSavedViews(views: SavedAppView[]): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(views))
  } catch {
    // Storage can be blocked or full; views are a convenience only.
  }
}

export function clearSavedViews(): void {
  try {
    window.localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Blocked storage has nothing to clear.
  }
}

export function filtersActive(f: AppListFilters): boolean {
  return f.tags.length > 0 || f.environments.length > 0 || f.query !== ''
}
