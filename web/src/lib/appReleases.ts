import type { AppRelease } from '../types/appRelease'

export interface ReleaseRow {
  release: AppRelease
  isCurrent: boolean
}

// Newest first; ties keep API order via the stable sort.
export function buildReleaseRows(
  releases: AppRelease[],
  currentId?: string,
): ReleaseRow[] {
  return [...releases]
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))
    .map((release) => ({
      release,
      isCurrent: !!currentId && release.id === currentId,
    }))
}
