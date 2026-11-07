import type * as React from 'react'
import {
  ActivityGlassIcon,
  ArchiveGlassIcon,
  BellGlassIcon,
  BinaryTreeGlassIcon,
  BookOpenGlassIcon,
  BotGlassIcon,
  BracesGlassIcon,
  BugGlassIcon,
  ChartLineGlassIcon,
  ClipboardListGlassIcon,
  CloudGlassIcon,
  CoinsGlassIcon,
  CountdownGlassIcon,
  CpuGlassIcon,
  DatabaseGlassIcon,
  EyeGlassIcon,
  FileSearchGlassIcon,
  FlagGlassIcon,
  FlaskConicalGlassIcon,
  FolderGlassIcon,
  FunnelGlassIcon,
  GaugeGlassIcon,
  GavelGlassIcon,
  GitBranchGlassIcon,
  GitPullRequestGlassIcon,
  GlobeGlassIcon,
  ImagesGlassIcon,
  InboxGlassIcon,
  KeyGlassIcon,
  LayersGlassIcon,
  LayoutGridGlassIcon,
  LightbulbGlassIcon,
  ListGlassIcon,
  LockGlassIcon,
  PackageGlassIcon,
  PlugGlassIcon,
  RestoreGlassIcon,
  RocketGlassIcon,
  ScrollGlassIcon,
  ServerGlassIcon,
  SettingsGlassIcon,
  Share2GlassIcon,
  ShieldAlertGlassIcon,
  ShieldCheckGlassIcon,
  SignpostGlassIcon,
  SlidersHorizontalGlassIcon,
  SplitGlassIcon,
  MailGlassIcon,
  TerminalGlassIcon,
  TrafficConeGlassIcon,
  UploadGlassIcon,
  WrenchGlassIcon,
  ZapGlassIcon,
} from '@thesvg/ui-react'
import { GO_TARGETS } from '@/lib/shortcuts'
import type { ExperimentalFeature } from '@/lib/experimental'

export type GlobalTo =
  | '/'
  | '/status'
  | '/apps'
  | '/deployments'
  | '/functions'
  | '/deploy-timeline'
  | '/images'
  | '/storage'
  | '/builds'
  | '/projects'
  | '/templates'
  | '/tags'
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

export const APPS_GROUP_PATHS = ['/projects', '/templates', '/tags'] as const
export const PIPELINE_GROUP_PATHS = ['/pipelines', '/approvals'] as const
export const DOMAIN_GROUP_PATHS = ['/domains', '/dns', '/mail'] as const
export const TRAFFIC_GROUP_PATHS = [
  '/network/proxy',
  '/loadbalancers',
  '/cdn',
  '/firewall',
] as const
export const REGISTRY_GROUP_PATHS = [
  '/images',
  '/settings/registry',
  '/settings/registry-credentials',
] as const

export const GLOBAL_NAV_GROUPS: GlobalNavGroup[] = [
  {
    id: 'primary',
    label: 'Home',
    ungrouped: true,
    items: [
      {
        id: 'dashboard',
        label: 'Dashboard',
        to: '/',
        icon: <GaugeGlassIcon size="1.4em" />,
        exact: true,
        // Status is reached from the dashboard status strip.
        alsoActive: ['/status'],
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
        icon: <LayersGlassIcon size="1.4em" />,
        badge: 'failing-apps',
        alsoActive: APPS_GROUP_PATHS,
      },
      {
        id: 'deployments',
        label: 'Deployments',
        to: '/deployments',
        icon: <RocketGlassIcon size="1.4em" />,
        alsoActive: DEPLOYMENT_GROUP_PATHS,
      },
      {
        id: 'pipelines',
        label: 'Pipelines',
        to: '/pipelines',
        icon: <BinaryTreeGlassIcon size="1.4em" />,
        badge: 'approvals',
        alsoActive: PIPELINE_GROUP_PATHS,
      },
      {
        id: 'functions',
        label: 'Functions',
        to: '/functions',
        icon: <ZapGlassIcon size="1.4em" />,
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
        icon: <DatabaseGlassIcon size="1.4em" />,
        alsoActive: ['/backups'],
      },
      {
        id: 'storage',
        label: 'Storage',
        to: '/storage',
        icon: <ArchiveGlassIcon size="1.4em" />,
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
        icon: <GlobeGlassIcon size="1.4em" />,
        badge: 'domains-attention',
        extraBadges: ['dns-attention'],
        alsoActive: DOMAIN_GROUP_PATHS,
      },
      {
        id: 'proxy',
        label: 'Traffic',
        to: '/network/proxy',
        icon: <TrafficConeGlassIcon size="1.4em" />,
        badge: 'proxy-attention',
        alsoActive: TRAFFIC_GROUP_PATHS,
      },
      {
        id: 'private-networks',
        label: 'Private network',
        to: '/network/private',
        icon: <LockGlassIcon size="1.4em" />,
        // Topology is exactly /network; a prefix match would also light Traffic's pages.
        alsoExact: ['/network'],
      },
    ],
  },
  {
    id: 'infrastructure',
    label: 'Infrastructure',
    items: [
      {
        id: 'nodes',
        label: 'Servers',
        to: '/nodes',
        icon: <ServerGlassIcon size="1.4em" />,
        alsoActive: ['/settings/node-providers'],
      },
      {
        id: 'alerts',
        label: 'Alerts',
        to: '/alerts',
        icon: <BellGlassIcon size="1.4em" />,
      },
      {
        id: 'registry',
        label: 'Registry',
        to: '/images',
        icon: <PackageGlassIcon size="1.4em" />,
        alsoActive: REGISTRY_GROUP_PATHS,
      },
      {
        id: 'usage',
        label: 'Usage and cost',
        to: '/usage',
        icon: <CoinsGlassIcon size="1.4em" />,
        alsoActive: COST_GROUP_PATHS,
      },
    ],
  },
  {
    id: 'access',
    label: 'Security',
    items: [
      {
        id: 'security',
        label: 'Security',
        to: '/security',
        icon: (
          <ShieldAlertGlassIcon className="text-tone-warning" size="1.4em" />
        ),
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
  folded('status', 'Status', '/status', <ActivityGlassIcon size="1.4em" />),
  folded('projects', 'Projects', '/projects', <FolderGlassIcon size="1.4em" />),
  folded(
    'templates',
    'Templates',
    '/templates',
    <LayoutGridGlassIcon size="1.4em" />,
  ),
  folded('tags', 'Tags', '/tags', <ListGlassIcon size="1.4em" />),
  folded(
    'approvals',
    'Deploy approvals',
    '/approvals',
    <GavelGlassIcon size="1.4em" />,
  ),
  folded('backups', 'Backups', '/backups', <UploadGlassIcon size="1.4em" />),
  folded(
    'network-shares',
    'Network shares',
    '/settings/network-shares',
    <FolderGlassIcon size="1.4em" />,
  ),
  folded('assets', 'Assets', '/assets', <ImagesGlassIcon size="1.4em" />),
  folded('dns', 'DNS', '/dns', <ListGlassIcon size="1.4em" />, {
    requiresRoute: true,
  }),
  folded('mail', 'Mail', '/mail', <MailGlassIcon size="1.4em" />, {
    feature: 'mail',
  }),
  folded(
    'firewall',
    'Firewall',
    '/firewall',
    <ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />,
  ),
  folded(
    'loadbalancers',
    'Load balancers',
    '/loadbalancers',
    <SplitGlassIcon size="1.4em" />,
    { feature: 'load-balancer' },
  ),
  folded('cdn', 'CDN and caches', '/cdn', <GlobeGlassIcon size="1.4em" />),
  folded('network', 'Topology', '/network', <Share2GlassIcon size="1.4em" />, {
    exact: true,
  }),
  folded(
    'node-providers',
    'Cloud providers',
    '/settings/node-providers',
    <CloudGlassIcon size="1.4em" />,
  ),
  folded(
    'registry-settings',
    'Registry settings',
    '/settings/registry',
    <PackageGlassIcon size="1.4em" />,
  ),
  folded(
    'registry-credentials',
    'Registry credentials',
    '/settings/registry-credentials',
    <KeyGlassIcon size="1.4em" />,
  ),
  folded(
    'resource-advisor',
    'Resource advisor',
    '/resource-advisor',
    <LightbulbGlassIcon size="1.4em" />,
    { feature: 'resource-advisor' },
  ),
  folded(
    'resources',
    'Resources',
    '/resources',
    <FileSearchGlassIcon size="1.4em" />,
  ),
  folded(
    'iam-policies',
    'IAM policies',
    '/settings/iam-policies',
    <ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />,
  ),
  folded(
    'scanning',
    'Image scanning',
    '/security/image-scanning',
    <ShieldAlertGlassIcon size="1.4em" />,
  ),
  folded(
    'bootstrap',
    'Bootstrap scripts',
    '/nodes/bootstrap-scripts',
    <TerminalGlassIcon size="1.4em" />,
  ),
  folded('inbox', 'Inbox', '/inbox', <InboxGlassIcon size="1.4em" />),
  folded('models', 'AI models', '/models', <CpuGlassIcon size="1.4em" />, {
    feature: 'ai-models',
  }),
  folded(
    'assistant',
    'AI assistant',
    '/ai-assistant',
    <BotGlassIcon size="1.4em" />,
    { feature: 'ai-chat' },
  ),
  folded(
    'console',
    'Console',
    '/terminal',
    <TerminalGlassIcon size="1.4em" />,
    { feature: 'cli-console' },
  ),
]

export const GLOBAL_NAV_FOOTER: GlobalNavItem[] = [
  {
    id: 'settings',
    label: 'Settings',
    to: '/settings',
    icon: <SettingsGlassIcon size="1.4em" />,
  },
  {
    id: 'help',
    label: 'Help',
    to: '/help',
    icon: <BookOpenGlassIcon size="1.4em" />,
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
    icon: <RocketGlassIcon size="1.4em" />,
    items: [
      item(
        'overview',
        'Overview',
        '/apps/$name/overview',
        <LayoutGridGlassIcon size="1.4em" />,
      ),
      item(
        'deploys',
        'Deploys',
        '/apps/$name/deploys',
        <RestoreGlassIcon size="1.4em" />,
      ),
      item(
        'source',
        'Source',
        '/apps/$name/source',
        <GitBranchGlassIcon size="1.4em" />,
      ),
      item(
        'previews',
        'Previews',
        '/apps/$name/previews',
        <GitPullRequestGlassIcon size="1.4em" />,
      ),
      item(
        'deploy-settings',
        'Deploy settings',
        '/apps/$name/deploy-settings',
        <SlidersHorizontalGlassIcon size="1.4em" />,
      ),
      item(
        'pipelines',
        'Pipelines',
        '/apps/$name/pipelines',
        <BinaryTreeGlassIcon size="1.4em" />,
      ),
    ],
  },
  {
    id: 'traffic',
    label: 'Traffic',
    icon: <Share2GlassIcon size="1.4em" />,
    items: [
      item(
        'go-live',
        'Go live',
        '/apps/$name/go-live',
        <RocketGlassIcon size="1.4em" />,
      ),
      item(
        'domains',
        'Domains',
        '/apps/$name/domains',
        <GlobeGlassIcon size="1.4em" />,
      ),
      item(
        'streams',
        'Streams',
        '/apps/$name/streams',
        <PlugGlassIcon size="1.4em" />,
      ),
      item(
        'loadbalancer',
        'Load balancer',
        '/apps/$name/loadbalancer',
        <SplitGlassIcon size="1.4em" />,
        'load-balancer',
      ),
      item(
        'network',
        'Network',
        '/apps/$name/network',
        <Share2GlassIcon size="1.4em" />,
      ),
      item(
        'cache',
        'Cache',
        '/apps/$name/cache',
        <ZapGlassIcon size="1.4em" />,
      ),
      {
        ...item(
          'forms',
          'Forms',
          '/apps/$name/forms',
          <ClipboardListGlassIcon size="1.4em" />,
        ),
        staticOnly: true,
        beta: true,
      },
      {
        ...item(
          'static-routes',
          'Routing rules',
          '/apps/$name/static-routes',
          <SignpostGlassIcon size="1.4em" />,
        ),
        staticOnly: true,
      },
    ],
  },
  {
    id: 'config',
    label: 'Config',
    icon: <WrenchGlassIcon size="1.4em" />,
    items: [
      item(
        'environment',
        'Environment',
        '/apps/$name/environment',
        <BracesGlassIcon size="1.4em" />,
      ),
      item(
        'services',
        'Services',
        '/apps/$name/services',
        <LayersGlassIcon size="1.4em" />,
      ),
      item(
        'resources',
        'Resources',
        '/apps/$name/resources',
        <CpuGlassIcon size="1.4em" />,
      ),
      item(
        'volumes',
        'Volumes',
        '/apps/$name/volumes',
        <ServerGlassIcon size="1.4em" />,
      ),
      item(
        'health',
        'Health',
        '/apps/$name/health',
        <ActivityGlassIcon size="1.4em" />,
      ),
      item(
        'scheduled-tasks',
        'Jobs',
        '/apps/$name/scheduled-tasks',
        <CountdownGlassIcon size="1.4em" />,
      ),
      item(
        'feature-flags',
        'Feature flags',
        '/apps/$name/feature-flags',
        <FlagGlassIcon size="1.4em" />,
      ),
      item(
        'split-testing',
        'Split testing',
        '/apps/$name/split-testing',
        <FlaskConicalGlassIcon size="1.4em" />,
      ),
      item(
        'integrations',
        'Integrations',
        '/apps/$name/integrations',
        <PlugGlassIcon size="1.4em" />,
      ),
    ],
  },
  {
    id: 'observe',
    label: 'Observe',
    icon: <EyeGlassIcon size="1.4em" />,
    items: [
      item(
        'metrics',
        'Metrics',
        '/apps/$name/metrics',
        <ActivityGlassIcon size="1.4em" />,
      ),
      item(
        'logs',
        'Logs',
        '/apps/$name/logs',
        <ScrollGlassIcon size="1.4em" />,
      ),
      item(
        'log-pipeline',
        'Log pipeline',
        '/apps/$name/log-pipeline',
        <FunnelGlassIcon size="1.4em" />,
      ),
      item(
        'requests',
        'Requests',
        '/apps/$name/requests',
        <FileSearchGlassIcon size="1.4em" />,
      ),
      item(
        'traffic',
        'Traffic',
        '/apps/$name/traffic',
        <ChartLineGlassIcon size="1.4em" />,
      ),
      item(
        'traces',
        'Traces',
        '/apps/$name/traces',
        <BinaryTreeGlassIcon size="1.4em" />,
      ),
      item(
        'errors',
        'Errors',
        '/apps/$name/errors',
        <BugGlassIcon size="1.4em" />,
      ),
      item(
        'alerts',
        'Alerts',
        '/apps/$name/alerts',
        <BellGlassIcon size="1.4em" />,
      ),
      item(
        'exec',
        'Exec',
        '/apps/$name/exec',
        <TerminalGlassIcon size="1.4em" />,
      ),
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

// With about 15 rows every group starts open.
// Anything the user toggles is persisted and wins from then on.
export function defaultGroupOpen(): boolean {
  return true
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
