import { useQuery } from '@tanstack/react-query'
import { siteChecksQueryOptions } from '../../queries/pagesSites'
import { useStaticRoutes } from '../../queries/staticRoutes'
import { useApp } from '../../queries/apps'
import type { PageTab } from './pageHelpers'

export interface TabBadge {
  count?: number
  alert?: boolean
}

/** Counts and the failing-check flag come from queries the tabs already use, so nothing is invented. */
export function usePageTabBadges(
  name: string,
): Partial<Record<PageTab, TabBadge>> {
  const { data: app } = useApp(name)
  const routes = useStaticRoutes(name).data
  const checks = useQuery(siteChecksQueryOptions(name)).data
  const out: Partial<Record<PageTab, TabBadge>> = {}
  const domains = app.domains?.length ?? 0
  if (domains > 0) out.domains = { count: domains }
  if (routes && routes.rules.length > 0) {
    out.routes = { count: routes.rules.length }
  }
  if (checks?.checks.some((c) => c.status === 'fail')) {
    out.overview = { alert: true }
  }
  return out
}
