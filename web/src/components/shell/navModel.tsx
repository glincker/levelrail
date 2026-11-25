import type * as React from 'react'
import { NavGlassIcon, type NavIconName } from './NavGlassIcon'
import { GO_TARGETS } from '@/lib/shortcuts'
import type { ExperimentalFeature } from '@/lib/experimental'

const glass = (name: NavIconName) => <NavGlassIcon name={name} />

export type GlobalTo =
  | '/'
  | '/status'
  | '/apps'
  | '/deployments'
  | '/functions'
  | '/pages'
  | '/deploy-timeline'
  | '/images'
  | '/storage'
  | '/builds'
  | '/projects'
  | '/templates'
  | '/tags'
  | '/variables'
  | '/sources'
  | '/databases'
  | '/backups'
  | '/assets'
  | '/nodes'
  | '/resource-advisor'
  | '/usage'
  | '/costs'
  | '/resources'
  | '/network'
  | '/network/proxy'
  | '/network/private'
  | '/cdn'
  | '/domains'
  | '/domains/certificates'
  | '/settings/notification-channels'
  | '/dns'
  | '/loadbalancers'
  | '/mail'
  | '/firewall'
  | '/models'
  | '/ai-assistant'
  | '/terminal'
  | '/pipelines'
  | '/inbox'
  | '/approvals'
  | '/alerts'
  | '/settings'
  | '/settings/iam-policies'
  | '/security'
  | '/security/image-scanning'
  | '/nodes/bootstrap-scripts'
  | '/clusters'
  | '/integrations'
  | '/settings/tokens'
  | '/settings/users'
  | '/settings/audit-log'
  | '/settings/registry'
  | '/settings/registry-credentials'
  | '/settings/network-shares'
  | '/settings/node-providers'
  | '/help'

// Routes whose pages land in a later change: not in the generated route tree
// yet, so they are gated by requiresRoute and linked through linkTarget().
export type PendingTo = '/dns'

export type GlobalBadge =
  | 'failing-apps'
  | 'approvals'
  | 'inbox'
  | 'domains-attention'
  | 'dns-attention'
  | 'proxy-attention'

export interface GlobalNavItem {
  id: string
  label: string
  to: GlobalTo | PendingTo
  icon: React.ReactNode
  exact?: boolean
  // Sibling pages shown as tabs under this entry: also mark it active.
  alsoActive?: readonly string[]
  badge?: GlobalBadge
  // Further counts summed into the badge when this row stands in for folded pages.
  extraBadges?: readonly GlobalBadge[]
  // Exact paths that also light this row (a prefix would over-match).
  alsoExact?: readonly string[]
  // Hidden from the sidebar until the user has data; stays in palette and routes.
  showWhen?: 'has-functions'
  feature?: ExperimentalFeature
  // Hidden until the router actually has this route (it lands in another PR).
  requiresRoute?: boolean
}

export interface GlobalNavGroup {
  id: string
  label: string
  // Primary links render flat with no header and are never collapsed.
  ungrouped?: boolean
  items: GlobalNavItem[]
}

// Pages reached through a SectionTabs strip instead of their own sidebar entry.
export const DEPLOYMENT_GROUP_PATHS = [
  '/deployments',
  '/deploy-timeline',
  '/builds',
] as const

export const COST_GROUP_PATHS = [
  '/usage',
  '/costs',
  '/resource-advisor',
  '/resources',
] as const

export const APPS_GROUP_PATHS = ['/tags', '/variables', '/sources'] as const
export const PIPELINE_GROUP_PATHS = ['/pipelines', '/approvals'] as const
export const DOMAIN_GROUP_PATHS = [
  '/domains',
  '/dns',
  '/domains/certificates',
] as const
export const TRAFFIC_GROUP_PATHS = [
  '/network/proxy',
  '/network/private',
  '/loadbalancers',
  '/cdn',
] as const
export const REGISTRY_GROUP_PATHS = [
  '/images',
  '/settings/registry',
  '/settings/registry-credentials',
] as const

// Cloud providers move under Integrations; /clusters stays a Servers tab.
export const INTEGRATIONS_GROUP_PATHS = [
  '/settings/integrations',
  '/settings/git-connections',
  '/settings/node-providers',
] as const

export const GLOBAL_NAV_GROUPS: GlobalNavGroup[] = [
  {
    id: 'primary',
    label: 'Overview',
    ungrouped: true,
    items: [
      {
        id: 'dashboard',
        label: 'Dashboard',
        to: '/',
        icon: glass('Gauge'),
        exact: true,
        // Status is reached from the dashboard status strip.
        alsoActive: ['/status'],
      },
      {
        id: 'projects',
        label: 'Projects',
        to: '/projects',
        icon: glass('Folder'),
      },
      {
        id: 'inbox',
        label: 'Inbox',
        to: '/inbox',
        icon: glass('Inbox'),
        badge: 'inbox',
      },
      {
        id: 'alerts',
        label: 'Alerts',
        to: '/alerts',
        icon: glass('Bell'),
      },
    ],
  },
  {
    id: 'build',
    label: 'Build',
    items: [
      {
        id: 'apps',
        label: 'Apps',
        to: '/apps',
        icon: glass('Layers'),
        badge: 'failing-apps',
        alsoActive: APPS_GROUP_PATHS,
      },
      {
        id: 'templates',
        label: 'Templates',
        to: '/templates',
        icon: glass('LayoutGrid'),
      },
      {
        id: 'pages',
        label: 'Pages',
        to: '/pages',
        icon: glass('LayoutTemplate'),
      },
      {
        id: 'deployments',
        label: 'Deployments',
        to: '/deployments',
        icon: glass('Rocket'),
        alsoActive: DEPLOYMENT_GROUP_PATHS,
      },
      {
        id: 'pipelines',
        label: 'Pipelines',
        to: '/pipelines',
        icon: glass('BinaryTree'),
        badge: 'approvals',
        alsoActive: PIPELINE_GROUP_PATHS,
      },
      {
        id: 'functions',
        label: 'Functions',
        to: '/functions',
        icon: glass('Zap'),
        showWhen: 'has-functions',
      },
    ],
  },
  {
    id: 'data',
    label: 'Data',
    items: [
      {
        id: 'databases',
        label: 'Databases',
        to: '/databases',
        icon: glass('Database'),
      },
      {
        id: 'backups',
        label: 'Backups',
        to: '/backups',
        icon: glass('Upload'),
      },
      {
        id: 'storage',
        label: 'Storage',
        to: '/storage',
        icon: glass('Archive'),
        alsoActive: ['/settings/network-shares', '/assets'],
      },
    ],
  },
  {
    id: 'traffic',
    label: 'Network',
    items: [
      {
        id: 'domains',
        label: 'Domains',
        to: '/domains',
        icon: glass('Globe'),
        badge: 'domains-attention',
        extraBadges: ['dns-attention'],
        alsoActive: DOMAIN_GROUP_PATHS,
      },
      {
        id: 'proxy',
        label: 'Traffic',
        to: '/network/proxy',
        icon: glass('TrafficCone'),
        badge: 'proxy-attention',
        alsoActive: TRAFFIC_GROUP_PATHS,
        // Topology is exactly /network; a prefix match would also light other pages.
        alsoExact: ['/network'],
      },
      {
        id: 'firewall',
        label: 'Firewall',
        to: '/firewall',
        icon: <NavGlassIcon name="ShieldCheck" />,
      },
      {
        id: 'mail',
        label: 'Mail',
        to: '/mail',
        icon: glass('Mail'),
        feature: 'mail',
      },
    ],
  },
  {
    id: 'platform',
    label: 'Platform',
    items: [
      {
        id: 'nodes',
        label: 'Servers',
        to: '/nodes',
        icon: glass('Server'),
        alsoActive: ['/clusters'],
      },
      {
        id: 'integrations',
        label: 'Integrations',
        to: '/integrations',
        icon: glass('Plug'),
        alsoActive: INTEGRATIONS_GROUP_PATHS,
      },
      {
        id: 'registry',
        label: 'Registry',
        // /images redirects into the Storage hub, so it cannot be the row target.
        to: '/settings/registry',
        icon: glass('Package'),
        alsoActive: REGISTRY_GROUP_PATHS,
      },
      {
        id: 'usage',
        label: 'Usage and cost',
        to: '/usage',
        icon: glass('Coins'),
        alsoActive: COST_GROUP_PATHS,
      },
      {
        id: 'security',
        label: 'Security',
        to: '/security',
        icon: <NavGlassIcon name="ShieldAlert" />,
        alsoActive: ['/settings/iam-policies'],
      },
    ],
  },
]

const folded = (
  id: string,
  label: string,
  to: GlobalTo | PendingTo,
  icon: React.ReactNode,
  extra: Partial<GlobalNavItem> = {},
): GlobalNavItem => ({ id, label, to, icon, ...extra })

// Destinations folded into a parent row's tab strip. No sidebar row, but they
// stay in the command palette and light their parent through alsoActive.
export const FOLDED_GLOBAL_PAGES: GlobalNavItem[] = [
  folded(
    'private-networks',
    'Private network',
    '/network/private',
    glass('Lock'),
  ),
  folded('status', 'Status', '/status', glass('Activity')),
  folded('tags', 'Tags', '/tags', glass('List')),
  folded('variables', 'Variables', '/variables', glass('Braces')),
  folded('sources', 'Sources', '/sources', glass('GitBranch')),
  folded('approvals', 'Deploy approvals', '/approvals', glass('Gavel')),
  folded(
    'network-shares',
    'Network shares',
    '/settings/network-shares',
    glass('Folder'),
  ),
  folded('assets', 'Assets', '/assets', glass('Images')),
  folded('dns', 'DNS', '/dns', glass('List'), {
    requiresRoute: true,
  }),
  folded('loadbalancers', 'Load balancers', '/loadbalancers', glass('Split'), {
    feature: 'load-balancer',
  }),
  folded('cdn', 'CDN and caches', '/cdn', glass('Globe')),
  folded('network', 'Topology', '/network', glass('Share2'), {
    exact: true,
  }),
  folded(
    'node-providers',
    'Cloud providers',
    '/settings/node-providers',
    glass('Cloud'),
  ),
  folded(
    'registry-credentials',
    'Registry credentials',
    '/settings/registry-credentials',
    glass('Key'),
  ),
  folded(
    'resource-advisor',
    'Resource advisor',
    '/resource-advisor',
    glass('Lightbulb'),
    { feature: 'resource-advisor' },
  ),
  folded('resources', 'Resources', '/resources', glass('FileSearch')),
  folded(
    'iam-policies',
    'IAM policies',
    '/settings/iam-policies',
    <NavGlassIcon name="ShieldCheck" />,
  ),
  folded(
    'scanning',
    'Image scanning',
    '/security/image-scanning',
    glass('ShieldAlert'),
  ),
  folded('clusters', 'Clusters', '/clusters', glass('Boxes')),
  folded(
    'bootstrap',
    'Bootstrap scripts',
    '/nodes/bootstrap-scripts',
    glass('Terminal'),
  ),
  folded('models', 'AI models', '/models', glass('Cpu'), {
    feature: 'ai-models',
  }),
  folded('assistant', 'AI assistant', '/ai-assistant', glass('Bot'), {
    feature: 'ai-chat',
  }),
  folded('console', 'Console', '/terminal', glass('Terminal'), {
    feature: 'cli-console',
  }),
]

export const GLOBAL_NAV_FOOTER: GlobalNavItem[] = [
  {
    id: 'settings',
    label: 'Settings',
    to: '/settings',
    icon: glass('Settings'),
  },
  {
    id: 'help',
    label: 'Help',
    to: '/help',
    icon: glass('BookOpen'),
  },
]

// Gated items stay in place; the renderer shows them locked (see
// isFeatureLocked) so a switched-off feature is discoverable, not hidden.
export function visibleGlobalGroups(
  routeAvailable: (to: string) => boolean = () => true,
): GlobalNavGroup[] {
  return GLOBAL_NAV_GROUPS.map((g) => ({
    ...g,
    items: g.items.filter((i) => !i.requiresRoute || routeAvailable(i.to)),
  })).filter((g) => g.items.length > 0)
}

// A group with one item renders as that item, so no header repeats its label.
export function isSingleItemGroup(group: {
  items: readonly unknown[]
}): boolean {
  return group.items.length === 1
}

export function visibleGlobalFooter(): GlobalNavItem[] {
  return GLOBAL_NAV_FOOTER
}

export function visibleAppSections(isStatic = false): AppNavSection[] {
  if (isStatic) return APP_NAV_SECTIONS
  return APP_NAV_SECTIONS.map((s) => ({
    ...s,
    items: s.items.filter((i) => !i.staticOnly),
  }))
}

export function allGlobalItems(): GlobalNavItem[] {
  return [
    ...GLOBAL_NAV_GROUPS.flatMap((g) => g.items),
    ...FOLDED_GLOBAL_PAGES,
    ...GLOBAL_NAV_FOOTER,
  ]
}

export function itemBadgeCount(
  item: GlobalNavItem,
  counts: Record<string, number>,
): number {
  return [item.badge, ...(item.extraBadges ?? [])].reduce(
    (n, b) => n + (b ? (counts[b] ?? 0) : 0),
    0,
  )
}

// A row with no data yet stays out of the way unless the user is on its page.
export function isItemDisclosed(
  item: GlobalNavItem,
  counts: Record<string, number>,
  pathname: string,
): boolean {
  if (item.showWhen === 'has-functions') {
    return (counts.functions ?? 0) > 0 || isGlobalItemActive(pathname, item)
  }
  return true
}

export function isGlobalItemActive(
  pathname: string,
  item: GlobalNavItem,
): boolean {
  if (item.exact) {
    if (pathname === item.to) return true
  } else if (pathname === item.to || pathname.startsWith(`${item.to}/`)) {
    return true
  }
  if ((item.alsoExact ?? []).includes(pathname)) return true
  return (item.alsoActive ?? []).some((p) => matchesPrefix(pathname, p))
}

// One active row per path: the most specific match wins, so a hub parent that
// lists child pages as tabs (Apps) yields to the child row (Projects).
export function activeGlobalItemId(
  pathname: string,
  items: readonly GlobalNavItem[],
): string | undefined {
  let best: { id: string; score: number } | undefined
  for (const item of items) {
    if (!isGlobalItemActive(pathname, item)) continue
    const own = item.exact
      ? pathname === item.to
      : pathname === item.to || pathname.startsWith(`${item.to}/`)
    const alsoLen = Math.max(
      0,
      ...(item.alsoActive ?? [])
        .filter((p) => matchesPrefix(pathname, p))
        .map((p) => p.length),
      ...(item.alsoExact ?? [])
        .filter((p) => p === pathname)
        .map((p) => p.length),
    )
    const score = own ? 10000 + String(item.to).length : alsoLen
    if (!best || score > best.score) best = { id: item.id, score }
  }
  return best?.id
}

// Tolerates a trailing query or hash so callers can pass a raw location string.
export function matchesPrefix(path: string, prefix: string): boolean {
  return (
    path === prefix ||
    path.startsWith(`${prefix}/`) ||
    path.startsWith(`${prefix}?`) ||
    path.startsWith(`${prefix}#`)
  )
}

// Typed Link needs a route-tree path; pending routes are only rendered once
// the router has them, so the narrowing is safe at runtime.
export function linkTarget(item: GlobalNavItem): GlobalTo {
  return item.to
}

export function chordFor(to: string): string[] | undefined {
  const entry = Object.entries(GO_TARGETS).find(([, t]) => t.to === to)
  return entry ? ['g', entry[0]] : undefined
}

export type AppTo =
  | '/apps/$name/overview'
  | '/apps/$name/deploys'
  | '/apps/$name/delivery'
  | '/apps/$name/source'
  | '/apps/$name/previews'
  | '/apps/$name/deploy-settings'
  | '/apps/$name/pipelines'
  | '/apps/$name/domains'
  | '/apps/$name/go-live'
  | '/apps/$name/streams'
  | '/apps/$name/loadbalancer'
  | '/apps/$name/network'
  | '/apps/$name/cache'
  | '/apps/$name/forms'
  | '/apps/$name/static-routes'
  | '/apps/$name/environment'
  | '/apps/$name/services'
  | '/apps/$name/resources'
  | '/apps/$name/volumes'
  | '/apps/$name/health'
  | '/apps/$name/containers'
  | '/apps/$name/scheduled-tasks'
  | '/apps/$name/feature-flags'
  | '/apps/$name/split-testing'
  | '/apps/$name/integrations'
  | '/apps/$name/metrics'
  | '/apps/$name/logs'
  | '/apps/$name/log-pipeline'
  | '/apps/$name/traces'
  | '/apps/$name/errors'
  | '/apps/$name/requests'
  | '/apps/$name/traffic'
  | '/apps/$name/alerts'
  | '/apps/$name/exec'

export interface AppNavItem {
  id: string
  slug: string
  label: string
  to: AppTo
  icon: React.ReactNode
  feature?: ExperimentalFeature
  /** Shown only for build.type: static apps. */
  staticOnly?: boolean
  /** Shows the Beta badge next to the label. */
  beta?: boolean
}

export interface AppNavSection {
  id: string
  label: string
  icon: React.ReactNode
  items: AppNavItem[]
}

const item = (
  slug: string,
  label: string,
  to: AppTo,
  icon: React.ReactNode,
  feature?: ExperimentalFeature,
): AppNavItem => ({ id: slug, slug, label, to, icon, feature })

export const APP_NAV_SECTIONS: AppNavSection[] = [
  {
    id: 'deploy',
    label: 'Deploy',
    icon: glass('Rocket'),
    items: [
      item('overview', 'Overview', '/apps/$name/overview', glass('LayoutGrid')),
      item('deploys', 'Deploys', '/apps/$name/deploys', glass('Restore')),
      item('source', 'Source', '/apps/$name/source', glass('GitBranch')),
      item('delivery', 'Delivery', '/apps/$name/delivery', glass('Workflow')),
      item(
        'previews',
        'Previews',
        '/apps/$name/previews',
        glass('GitPullRequest'),
      ),
      item(
        'deploy-settings',
        'Deploy settings',
        '/apps/$name/deploy-settings',
        glass('SlidersHorizontal'),
      ),
      item(
        'pipelines',
        'Pipelines',
        '/apps/$name/pipelines',
        glass('BinaryTree'),
      ),
    ],
  },
  {
    id: 'traffic',
    label: 'Traffic',
    icon: glass('Share2'),
    items: [
      item('go-live', 'Go live', '/apps/$name/go-live', glass('Rocket')),
      item('domains', 'Domains', '/apps/$name/domains', glass('Globe')),
      item('streams', 'Streams', '/apps/$name/streams', glass('Plug')),
      item(
        'loadbalancer',
        'Load balancer',
        '/apps/$name/loadbalancer',
        glass('Split'),
        'load-balancer',
      ),
      item('network', 'Network', '/apps/$name/network', glass('Share2')),
      item('cache', 'Cache', '/apps/$name/cache', glass('Zap')),
      {
        ...item('forms', 'Forms', '/apps/$name/forms', glass('ClipboardList')),
        staticOnly: true,
        beta: true,
      },
      {
        ...item(
          'static-routes',
          'Routing rules',
          '/apps/$name/static-routes',
          glass('Signpost'),
        ),
        staticOnly: true,
      },
    ],
  },
  {
    id: 'config',
    label: 'Config',
    icon: glass('Wrench'),
    items: [
      item(
        'environment',
        'Environment',
        '/apps/$name/environment',
        glass('Braces'),
      ),
      item('services', 'Services', '/apps/$name/services', glass('Layers')),
      item('resources', 'Resources', '/apps/$name/resources', glass('Cpu')),
      item('volumes', 'Volumes', '/apps/$name/volumes', glass('Server')),
      item('health', 'Health', '/apps/$name/health', glass('Activity')),
      item(
        'containers',
        'Containers',
        '/apps/$name/containers',
        glass('Container'),
      ),
      item(
        'scheduled-tasks',
        'Jobs',
        '/apps/$name/scheduled-tasks',
        glass('Countdown'),
      ),
      item(
        'feature-flags',
        'Feature flags',
        '/apps/$name/feature-flags',
        glass('Flag'),
      ),
      item(
        'split-testing',
        'Split testing',
        '/apps/$name/split-testing',
        glass('FlaskConical'),
      ),
      item(
        'integrations',
        'Integrations',
        '/apps/$name/integrations',
        glass('Plug'),
      ),
    ],
  },
  {
    id: 'observe',
    label: 'Observe',
    icon: glass('Eye'),
    items: [
      item('metrics', 'Metrics', '/apps/$name/metrics', glass('Activity')),
      item('logs', 'Logs', '/apps/$name/logs', glass('Scroll')),
      item(
        'log-pipeline',
        'Log pipeline',
        '/apps/$name/log-pipeline',
        glass('Funnel'),
      ),
      item('requests', 'Requests', '/apps/$name/requests', glass('FileSearch')),
      item('traffic', 'Traffic', '/apps/$name/traffic', glass('ChartLine')),
      item('traces', 'Traces', '/apps/$name/traces', glass('BinaryTree')),
      item('errors', 'Errors', '/apps/$name/errors', glass('Bug')),
      item('alerts', 'Alerts', '/apps/$name/alerts', glass('Bell')),
      item('exec', 'Exec', '/apps/$name/exec', glass('Terminal')),
    ],
  },
]

export function appSlugFromPath(pathname: string): string | undefined {
  return /^\/apps\/[^/]+\/([^/]+)/.exec(pathname)?.[1]
}

export function activeAppSection(
  pathname: string,
): { section: AppNavSection; item: AppNavItem } | undefined {
  const slug = appSlugFromPath(pathname)
  if (!slug) return undefined
  for (const section of APP_NAV_SECTIONS) {
    const found = section.items.find((i) => i.slug === slug)
    if (found) return { section, item: found }
  }
  return undefined
}

export function isSingleSection(section: AppNavSection): boolean {
  return section.items.length === 1
}

// Tall viewports open every group; shorter ones open only the group holding the current page.
// Anything the user toggles is persisted and wins from then on.
const OPEN_ALL_MIN_HEIGHT =
  Number(import.meta.env.VITE_SIDEBAR_OPEN_ALL_MIN_HEIGHT) || 900

export function defaultGroupOpen(
  viewportHeight: number,
  containsActive: boolean,
): boolean {
  return viewportHeight >= OPEN_ALL_MIN_HEIGHT || containsActive
}

export function resolveOpen(
  stored: Record<string, boolean>,
  id: string,
  fallback: boolean,
): boolean {
  return stored[id] ?? fallback
}

// Pinned ids resolve against what is visible now, in pin order; ids for
// items that no longer exist are dropped. The nested Projects tree is not pinnable.
export function pinnedGlobalItems(
  groups: readonly GlobalNavGroup[],
  pins: readonly string[],
): GlobalNavItem[] {
  const byId = new Map(
    groups.flatMap((g) => g.items).map((i) => [i.id, i] as const),
  )
  return pins.flatMap((id) => {
    const found = byId.get(id)
    return found && found.id !== 'projects' ? [found] : []
  })
}
