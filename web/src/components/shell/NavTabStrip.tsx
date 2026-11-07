import { useTranslation } from 'react-i18next'
import { useRouterState } from '@tanstack/react-router'
import { isFeatureLocked } from '@/lib/experimental'
import { useExperimentalFeatures } from '@/hooks/useExperimental'
import { useRouteAvailable } from '@/lib/routeAvailability'
import { SectionTabs } from './SectionTabs'
import { matchesPrefix } from './navModel'
import { NAV_TAB_GROUPS, navTabGroupFor, type NavTabGroup } from './navTabs'

function GroupStrip({ group }: { group: NavTabGroup }) {
  const { t } = useTranslation()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const experimental = useExperimentalFeatures()
  const routeAvailable = useRouteAvailable()
  const items = group.tabs
    .filter((tab) => !tab.requiresRoute || routeAvailable(tab.to))
    .filter((tab) => !isFeatureLocked(tab.feature, experimental))
    .map((tab) => ({
      to: tab.to,
      label: t(`shell.navTabs.tab.${tab.id}`),
      active: matchesPrefix(pathname, tab.to),
    }))
  if (items.length < 2) return null
  return (
    <SectionTabs
      ariaLabel={t(`shell.navTabs.group.${group.id}`)}
      items={items}
    />
  )
}

/** Sibling-page strip for a folded sidebar row. With groupId it mounts in a page header; without, it follows the current path. */
export function NavTabStrip({ groupId }: { groupId?: string }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const group = groupId
    ? NAV_TAB_GROUPS.find((g) => g.id === groupId)
    : navTabGroupFor(pathname)
  if (!group) return null
  const bare = pathname.length > 1 ? pathname.replace(/\/$/, '') : pathname
  if (!groupId && group.inPagePaths?.includes(bare)) return null
  return <GroupStrip group={group} />
}
