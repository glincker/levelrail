import type { InfiniteData } from '@tanstack/react-query'
import type {
  Deployment,
  DeploymentEvent,
  DeploymentListPage,
} from '../types/deployment'
import { matchesFilters, type DeploymentFilters } from './deploymentFilters'
import { isInProgress } from './deploymentPresentation'

export type DeploymentPages = InfiniteData<DeploymentListPage, unknown>

function clearOtherLive(rows: Deployment[], dep: Deployment): Deployment[] {
  if (!dep.is_live) return rows
  let changed = false
  const next = rows.map((r) => {
    if (
      r.id !== dep.id &&
      r.is_live &&
      r.app === dep.app &&
      r.environment === dep.environment
    ) {
      changed = true
      return { ...r, is_live: false }
    }
    return r
  })
  return changed ? next : rows
}

export function pagesContain(data: DeploymentPages, id: string): boolean {
  return data.pages.some((p) => p.items.some((r) => r.id === id))
}

/** Replaces a row already in the loaded pages; unknown ids are left alone. */
export function replaceInPages(
  data: DeploymentPages,
  dep: Deployment,
): DeploymentPages {
  let found = false
  const pages = data.pages.map((p) => {
    const idx = p.items.findIndex((r) => r.id === dep.id)
    if (idx === -1) return { ...p, items: clearOtherLive(p.items, dep) }
    found = true
    const items = p.items.slice()
    items[idx] = dep
    return { ...p, items: clearOtherLive(items, dep) }
  })
  return found || dep.is_live ? { ...data, pages } : data
}

export function prependToPages(
  data: DeploymentPages,
  deps: Deployment[],
): DeploymentPages {
  const fresh = deps.filter((d) => !pagesContain(data, d.id))
  const first = data.pages[0]
  if (fresh.length === 0 || !first) return data
  const rest = data.pages.slice(1)
  return {
    ...data,
    pages: [{ ...first, items: [...fresh, ...first.items] }, ...rest],
  }
}

export interface EventOutcome {
  data: DeploymentPages
  /** A row that matches the filters but is not loaded yet, held back for the "N new" pill. */
  pending: Deployment | null
}

export function applyEventToPages(
  data: DeploymentPages,
  ev: DeploymentEvent,
  filters: DeploymentFilters,
  atTop: boolean,
): EventOutcome {
  const dep = ev.deployment
  if (pagesContain(data, dep.id)) {
    return { data: replaceInPages(data, dep), pending: null }
  }
  const settled = replaceInPages(data, dep)
  if (!matchesFilters(dep, filters)) return { data: settled, pending: null }
  if (atTop) return { data: prependToPages(settled, [dep]), pending: null }
  return { data: settled, pending: dep }
}

export function flattenPages(data: DeploymentPages | undefined): Deployment[] {
  return data ? data.pages.flatMap((p) => p.items) : []
}

/** Keeps the "Building now" lane to in-progress rows, newest first. */
export function applyEventToLane(
  rows: Deployment[],
  ev: DeploymentEvent,
): Deployment[] {
  const dep = ev.deployment
  const without = rows.filter((r) => r.id !== dep.id)
  if (!isInProgress(dep)) {
    return without.length === rows.length ? rows : without
  }
  const next = [...without, dep]
  next.sort((a, b) => b.started_at.localeCompare(a.started_at))
  return next
}
