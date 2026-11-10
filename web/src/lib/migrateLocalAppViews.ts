import {
  clearSavedViews,
  loadSavedViews,
  storeSavedViews,
  type SavedAppView,
} from './appViews'
import { sanitizeViewFilter, type ViewFilter } from './savedViews'

export interface LocalViewDraft {
  name: string
  filter: ViewFilter
}

export function localViewDraft(v: SavedAppView): LocalViewDraft {
  return {
    name: v.name.trim(),
    filter: sanitizeViewFilter('apps', {
      q: v.filters.query,
      env: v.filters.environments.join(','),
      tags: v.filters.tags.join(','),
    }),
  }
}

function sameFilter(a: ViewFilter, b: ViewFilter): boolean {
  const ka = Object.keys(a)
  return ka.length === Object.keys(b).length && ka.every((k) => a[k] === b[k])
}

// Moves browser-only Apps views to the server. A view leaves this browser only once
// the server holds it; a name the user already has with another filter stays local.
export async function migrateLocalAppViews(
  existing: ReadonlyMap<string, ViewFilter>,
  save: (draft: LocalViewDraft) => Promise<unknown>,
): Promise<number> {
  const local = loadSavedViews()
  if (local.length === 0) return 0
  const kept: SavedAppView[] = []
  let moved = 0
  for (const view of local) {
    const draft = localViewDraft(view)
    const onServer = existing.get(draft.name)
    if (onServer && sameFilter(onServer, draft.filter)) {
      moved += 1
      continue
    }
    if (draft.name === '' || onServer) {
      kept.push(view)
      continue
    }
    try {
      await save(draft)
      moved += 1
    } catch {
      kept.push(view)
    }
  }
  if (kept.length === 0) clearSavedViews()
  else if (moved > 0) storeSavedViews(kept)
  return moved
}
