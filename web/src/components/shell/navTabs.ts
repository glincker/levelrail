import type { ExperimentalFeature } from '@/lib/experimental'
import type { GlobalTo } from './navModel'

// Literal ids keep the shell.navTabs i18n keys type-checked.
export type NavTabId =
  | 'advisor'
  | 'approvals'
  | 'apps'
  | 'assets'
  | 'backups'
  | 'cdn'
  | 'costs'
  | 'credentials'
  | 'databases'
  | 'dns'
  | 'domains'
  | 'firewall'
  | 'iam'
  | 'images'
  | 'loadBalancers'
  | 'mail'
  | 'networkShares'
  | 'nodes'
  | 'pipelines'
  | 'projects'
  | 'providers'
  | 'proxy'
  | 'registry'
  | 'resources'
  | 'security'
  | 'storage'
  | 'tags'
  | 'templates'
  | 'usage'

export type NavTabGroupId =
  | 'apps'
  | 'pipelines'
  | 'databases'
  | 'storage'
  | 'domains'
  | 'traffic'
  | 'servers'
  | 'cost'
  | 'registry'
  | 'security'

export interface NavTab {
  id: NavTabId
  to: GlobalTo
  feature?: ExperimentalFeature
  // Hidden until the router has the route (DNS lands in another change).
  requiresRoute?: boolean
}

export interface NavTabGroup {
  id: NavTabGroupId
  tabs: readonly NavTab[]
  // Paths whose page already mounts this strip in its own header.
  inPagePaths?: readonly string[]
}

// One strip per folded sidebar row, so every folded page can reach its siblings.
export const NAV_TAB_GROUPS: readonly NavTabGroup[] = [
  {
    id: 'apps',
    tabs: [
      { id: 'apps', to: '/apps' },
      { id: 'projects', to: '/projects' },
      { id: 'templates', to: '/templates' },
      { id: 'tags', to: '/tags' },
    ],
  },
  {
    id: 'pipelines',
    tabs: [
      { id: 'pipelines', to: '/pipelines' },
      { id: 'approvals', to: '/approvals' },
    ],
  },
  {
    id: 'databases',
    tabs: [
      { id: 'databases', to: '/databases' },
      { id: 'backups', to: '/backups' },
    ],
  },
  {
    id: 'storage',
    tabs: [
      { id: 'storage', to: '/storage' },
      { id: 'assets', to: '/assets' },
      { id: 'networkShares', to: '/settings/network-shares' },
    ],
  },
  {
    id: 'domains',
    tabs: [
      { id: 'domains', to: '/domains' },
      { id: 'dns', to: '/dns', requiresRoute: true },
      { id: 'mail', to: '/mail', feature: 'mail' },
    ],
  },
  {
    id: 'traffic',
    tabs: [
      { id: 'proxy', to: '/network/proxy' },
      { id: 'loadBalancers', to: '/loadbalancers', feature: 'load-balancer' },
      { id: 'cdn', to: '/cdn' },
      { id: 'firewall', to: '/firewall' },
    ],
  },
  {
    id: 'servers',
    tabs: [
      { id: 'nodes', to: '/nodes' },
      { id: 'providers', to: '/settings/node-providers' },
    ],
  },
  {
    id: 'registry',
    tabs: [
      { id: 'images', to: '/images' },
      { id: 'registry', to: '/settings/registry' },
      { id: 'credentials', to: '/settings/registry-credentials' },
    ],
  },
  {
    id: 'cost',
    inPagePaths: ['/usage', '/costs'],
    tabs: [
      { id: 'usage', to: '/usage' },
      { id: 'costs', to: '/costs' },
      { id: 'advisor', to: '/resource-advisor', feature: 'resource-advisor' },
      { id: 'resources', to: '/resources' },
    ],
  },
  {
    id: 'security',
    tabs: [
      { id: 'security', to: '/security' },
      { id: 'iam', to: '/settings/iam-policies' },
    ],
  },
]

// The strip group whose tab is exactly this page; sub-pages keep their own chrome.
export function navTabGroupFor(pathname: string): NavTabGroup | undefined {
  const bare = pathname.length > 1 ? pathname.replace(/\/$/, '') : pathname
  return NAV_TAB_GROUPS.find((g) => g.tabs.some((t) => t.to === bare))
}
