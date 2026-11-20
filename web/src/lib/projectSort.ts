import type { ProjectResource } from '../types/projectDetail'

export const PROJECT_SORTS = ['name', 'newest', 'oldest'] as const
export type ProjectSort = (typeof PROJECT_SORTS)[number]

const SORT_KEY = 'levelrail.projects.sort'
const PINS_KEY = 'levelrail.projects.pins'

export function isProjectSort(v: string | null): v is ProjectSort {
  return PROJECT_SORTS.some((s) => s === v)
}

export function loadProjectSort(): ProjectSort {
  try {
    const v = localStorage.getItem(SORT_KEY)
    return isProjectSort(v) ? v : 'name'
  } catch {
    return 'name'
  }
}

export function storeProjectSort(sort: ProjectSort): void {
  try {
    localStorage.setItem(SORT_KEY, sort)
  } catch {
    // Storage can be blocked; the choice then lasts for this visit only.
  }
}

export function loadProjectPins(): string[] {
  try {
    const raw: unknown = JSON.parse(localStorage.getItem(PINS_KEY) ?? '[]')
    return Array.isArray(raw)
      ? raw.filter((x): x is string => typeof x === 'string')
      : []
  } catch {
    return []
  }
}

export function storeProjectPins(pins: readonly string[]): void {
  try {
    localStorage.setItem(PINS_KEY, JSON.stringify(pins))
  } catch {
    // Same as the sort: pins then last for this visit only.
  }
}

// Pinned projects always lead, each group ordered by the chosen sort.
export function sortProjects(
  projects: readonly ProjectResource[],
  sort: ProjectSort,
  pins: readonly string[],
): ProjectResource[] {
  const pinned = new Set(pins)
  const cmp = (a: ProjectResource, b: ProjectResource): number => {
    if (sort === 'name') return a.name.localeCompare(b.name)
    const diff = Date.parse(a.created_at) - Date.parse(b.created_at)
    return sort === 'newest' ? -diff : diff
  }
  return [...projects].sort(
    (a, b) => Number(pinned.has(b.id)) - Number(pinned.has(a.id)) || cmp(a, b),
  )
}
