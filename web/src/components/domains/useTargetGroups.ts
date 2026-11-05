import { useQuery } from '@tanstack/react-query'
import { groupedTargets, type TargetOption } from '../../lib/domainLifecycle'
import { appListQueryOptions } from '../../queries/apps'
import { staticSitesQueryOptions } from '../../queries/staticSites'

// Static sites live in their own list; a failed read just hides the group.
export function useTargetGroups(source: TargetOption, allowSame: boolean) {
  const { data: apps = [] } = useQuery(appListQueryOptions())
  const { data: sites = [] } = useQuery(staticSitesQueryOptions())
  return groupedTargets(apps, sites, source, allowSame)
}

export function firstTarget(groups: {
  apps: TargetOption[]
  sites: TargetOption[]
}): TargetOption | null {
  return groups.apps[0] ?? groups.sites[0] ?? null
}
