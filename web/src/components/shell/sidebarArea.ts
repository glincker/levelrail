import { appScopeName, databaseScopeName } from '../../lib/sidebarScope'
import { GLOBAL_NAV_GROUPS, isGlobalItemActive } from './navModel'

export type SidebarArea = 'global' | 'app' | 'database' | 'settings'

const SETTINGS_SCOPE_PATTERN = /^\/settings(\/|$)/

// Which sidebar a path shows. Pages the main nav lists (registry, IAM...) live
// under /settings but keep the main sidebar, so opening one never swaps it.
export function sidebarArea(pathname: string): SidebarArea {
  if (appScopeName(pathname) !== undefined) return 'app'
  if (databaseScopeName(pathname) !== undefined) return 'database'
  const ownedByGlobalNav = GLOBAL_NAV_GROUPS.some((g) =>
    g.items.some((i) => isGlobalItemActive(pathname, i)),
  )
  return SETTINGS_SCOPE_PATTERN.test(pathname) && !ownedByGlobalNav
    ? 'settings'
    : 'global'
}
