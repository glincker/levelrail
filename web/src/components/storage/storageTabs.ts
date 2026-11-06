import { parseFilter, type StorageFilter } from './itemRows'

// Search values from before the tabs went away: image chips and the cache tab.
const LEGACY_IMAGE_CHIPS = ['platform', 'other', 'unused', 'in_use'] as const

export interface StorageSearchInput {
  tab?: unknown
  filter?: unknown
  review?: unknown
  view?: unknown
  drawer?: unknown
}

export interface StorageSearchState {
  filter?: StorageFilter
  view?: 'app'
  drawer?: 'cleanup'
}

/** Maps old deep links (tab, image chip, review) onto the filter and drawer params. */
export function normalizeStorageSearch(
  search: StorageSearchInput,
): StorageSearchState {
  const raw = typeof search.filter === 'string' ? search.filter : undefined
  let filter = parseFilter(raw) ?? undefined
  if (!filter && LEGACY_IMAGE_CHIPS.some((c) => c === raw)) filter = 'images'
  if (!filter && search.tab === 'cache') filter = 'buildCache'
  const drawer =
    search.drawer === 'cleanup' ||
    search.review === '1' ||
    search.review === true
  return {
    ...(filter ? { filter } : {}),
    ...(search.view === 'app' ? { view: 'app' as const } : {}),
    ...(drawer ? { drawer: 'cleanup' as const } : {}),
  }
}
