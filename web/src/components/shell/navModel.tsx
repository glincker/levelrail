import type * as React from 'react'
import {
  ArrowsSplitIcon,
  ClipboardTextIcon,
  BellIcon,
  BookOpenIcon,
  BracketsCurlyIcon,
  BugIcon,
  ClockCountdownIcon,
  CloudArrowUpIcon,
  CloudIcon,
  CpuIcon,
  DatabaseIcon,
  FlagIcon,
  FlaskIcon,
  FolderIcon,
  FolderOpenIcon,
  GaugeIcon,
  GavelIcon,
  GearIcon,
  GitBranchIcon,
  GitPullRequestIcon,
  GlobeIcon,
  GlobeHemisphereWestIcon,
  LightningIcon,
  SignpostIcon,
  ListBulletsIcon,
  HardDrivesIcon,
  HeartbeatIcon,
  KeyIcon,
  PackageIcon,
  CubeIcon,
  PlugsConnectedIcon,
  PlugsIcon,
  PulseIcon,
  RobotIcon,
  TerminalWindowIcon,
  RocketLaunchIcon,
  ScrollIcon,
  LockKeyIcon,
  ShareNetworkIcon,
  ShieldCheckIcon,
  ShieldWarningIcon,
  SlidersHorizontalIcon,
  SquaresFourIcon,
  StackIcon,
  TerminalIcon,
  ChartLineUpIcon,
  ListMagnifyingGlassIcon,
  TreeStructureIcon,
  ClockCounterClockwiseIcon,
  EyeIcon,
  TrafficSignalIcon,
  WrenchIcon,
  FunnelIcon,
  LightbulbIcon,
  CoinsIcon,
  TrayIcon,
} from '@phosphor-icons/react/dist/ssr'
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
  | '/builds'
  | '/projects'
  | '/databases'
  | '/backups'
  | '/nodes'
  | '/resource-advisor'
  | '/usage'
  | '/costs'
  | '/resources'
  | '/dashboards'
  | '/network'
  | '/network/proxy'
  | '/network/private'
  | '/cdn'
  | '/domains'
  | '/dns'
  | '/loadbalancers'
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
  feature?: ExperimentalFeature
  // Hidden until the router actually has this route (it lands in another PR).
  requiresRoute?: boolean
}

export interface GlobalNavGroup {
  id: string
  label: string
  items: GlobalNavItem[]
}

// Pages reached through a SectionTabs strip instead of their own sidebar entry.
export const DEPLOYMENT_GROUP_PATHS = [
  '/deployments',
  '/deploy-timeline',
  '/builds',
] as const

export const COST_GROUP_PATHS = ['/usage', '/costs'] as const

export const GLOBAL_NAV_GROUPS: GlobalNavGroup[] = [
  {
    id: 'home',
    label: 'Home',
    items: [
      {
        id: 'dashboard',
        label: 'Dashboard',
        to: '/',
        icon: <GaugeIcon />,
        exact: true,
      },
      { id: 'status', label: 'Status', to: '/status', icon: <HeartbeatIcon /> },
      {
        id: 'dashboards',
        label: 'Dashboards',
        to: '/dashboards',
        icon: <SquaresFourIcon />,
      },
    ],
  },
  {
    id: 'apps',
    label: 'Apps',
    items: [
      {
        id: 'apps',
        label: 'Apps',
        to: '/apps',
        icon: <StackIcon />,
        badge: 'failing-apps',
      },
      {
        id: 'deployments',
        label: 'Deployments',
        to: '/deployments',
        icon: <RocketLaunchIcon />,
        alsoActive: DEPLOYMENT_GROUP_PATHS,
      },
      {
        id: 'functions',
        label: 'Functions',
        to: '/functions',
        icon: <LightningIcon />,
      },
      {
        id: 'projects',
        label: 'Projects',
        to: '/projects',
        icon: <FolderIcon />,
      },
      {
        id: 'images',
        label: 'Images',
        to: '/images',
        icon: <CubeIcon />,
      },
      {
        id: 'registry',
        label: 'Container registry',
        to: '/settings/registry',
        icon: <PackageIcon />,
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
        icon: <DatabaseIcon />,
      },
      {
        id: 'backups',
        label: 'Backups',
        to: '/backups',
        icon: <CloudArrowUpIcon />,
      },
      {
        id: 'network-shares',
        label: 'Network shares',
        to: '/settings/network-shares',
        icon: <FolderOpenIcon />,
      },
    ],
  },
  {
    id: 'traffic',
    label: 'Networking',
    items: [
      {
        id: 'domains',
        label: 'Domains',
        to: '/domains',
        icon: <GlobeIcon />,
        badge: 'domains-attention',
      },
      {
        id: 'dns',
        label: 'DNS',
        to: '/dns',
        icon: <ListBulletsIcon />,
        badge: 'dns-attention',
        requiresRoute: true,
      },
      {
        id: 'proxy',
        label: 'Proxy',
        to: '/network/proxy',
        icon: <TrafficSignalIcon />,
        badge: 'proxy-attention',
      },
      {
        id: 'private-networks',
        label: 'Private networks',
        to: '/network/private',
        icon: <LockKeyIcon />,
      },
      {
        id: 'firewall',
        label: 'Firewall',
        to: '/firewall',
        icon: <ShieldCheckIcon />,
      },
      {
        id: 'loadbalancers',
        label: 'Load balancers',
        to: '/loadbalancers',
        icon: <ArrowsSplitIcon />,
        feature: 'load-balancer',
      },
      {
        id: 'cdn',
        label: 'CDN and caches',
        to: '/cdn',
        icon: <GlobeHemisphereWestIcon />,
      },
    ],
  },
  {
    id: 'infrastructure',
    label: 'Fleet',
    items: [
      { id: 'nodes', label: 'Nodes', to: '/nodes', icon: <HardDrivesIcon /> },
      {
        id: 'network',
        label: 'Topology',
        to: '/network',
        // Proxy and Private networks live under /network/*, so prefix matching lit two rows.
        exact: true,
        icon: <ShareNetworkIcon />,
      },
      {
        id: 'node-providers',
        label: 'Cloud providers',
        to: '/settings/node-providers',
        icon: <CloudIcon />,
      },
      {
        id: 'resource-advisor',
        label: 'Resource advisor',
        to: '/resource-advisor',
        icon: <LightbulbIcon />,
        feature: 'resource-advisor',
      },
      {
        id: 'usage',
        label: 'Usage and cost',
        to: '/usage',
        icon: <CoinsIcon />,
        alsoActive: COST_GROUP_PATHS,
      },
      {
        id: 'resources',
        label: 'Resources',
        to: '/resources',
        icon: <ListMagnifyingGlassIcon />,
      },
    ],
  },
  {
    id: 'automation',
    label: 'Automation',
    items: [
      {
        id: 'pipelines',
        label: 'Pipelines',
        to: '/pipelines',
        icon: <TreeStructureIcon />,
      },
      {
        id: 'approvals',
        label: 'Deploy approvals',
        to: '/approvals',
        icon: <GavelIcon />,
        badge: 'approvals',
      },
      {
        id: 'inbox',
        label: 'Inbox',
        to: '/inbox',
        icon: <TrayIcon />,
        badge: 'inbox',
      },
      { id: 'alerts', label: 'Alerts', to: '/alerts', icon: <BellIcon /> },
    ],
  },
  {
    id: 'access',
    label: 'Security',
    items: [
      {
        id: 'security',
        label: 'Security center',
        to: '/security',
        icon: <ShieldWarningIcon />,
      },
      {
        id: 'iam-policies',
        label: 'IAM policies',
        to: '/settings/iam-policies',
        icon: <ShieldCheckIcon />,
      },
      {
        id: 'registry-credentials',
        label: 'Registry credentials',
        to: '/settings/registry-credentials',
        icon: <KeyIcon />,
      },
    ],
  },
  {
    id: 'ai',
    label: 'AI',
    items: [
      {
        id: 'models',
        label: 'AI models',
        to: '/models',
        icon: <CpuIcon />,
        feature: 'ai-models',
      },
    ],
  },
]

export const GLOBAL_NAV_FOOTER: GlobalNavItem[] = [
  {
    id: 'assistant',
    label: 'AI assistant',
    to: '/ai-assistant',
    icon: <RobotIcon />,
    feature: 'ai-chat',
  },
  {
    id: 'console',
    label: 'Console',
    to: '/terminal',
    icon: <TerminalWindowIcon />,
    feature: 'cli-console',
  },
  { id: 'settings', label: 'Settings', to: '/settings', icon: <GearIcon /> },
  { id: 'help', label: 'Help', to: '/help', icon: <BookOpenIcon /> },
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
  return [...GLOBAL_NAV_GROUPS.flatMap((g) => g.items), ...GLOBAL_NAV_FOOTER]
}

export function isGlobalItemActive(
  pathname: string,
  item: GlobalNavItem,
): boolean {
  if (item.exact) return pathname === item.to
  if (pathname === item.to || pathname.startsWith(`${item.to}/`)) return true
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
    id: 'overview',
    label: 'Overview',
    icon: <SquaresFourIcon />,
    items: [
      item('overview', 'Overview', '/apps/$name/overview', <SquaresFourIcon />),
    ],
  },
  {
    id: 'deployments',
    label: 'Deployments',
    icon: <RocketLaunchIcon />,
    items: [
      item(
        'deploys',
        'Deploys',
        '/apps/$name/deploys',
        <ClockCounterClockwiseIcon />,
      ),
      item('source', 'Source', '/apps/$name/source', <GitBranchIcon />),
      item(
        'previews',
        'Previews',
        '/apps/$name/previews',
        <GitPullRequestIcon />,
      ),
      item(
        'deploy-settings',
        'Deploy settings',
        '/apps/$name/deploy-settings',
        <SlidersHorizontalIcon />,
      ),
      item(
        'pipelines',
        'Pipelines',
        '/apps/$name/pipelines',
        <TreeStructureIcon />,
      ),
    ],
  },
  {
    id: 'traffic',
    label: 'Traffic',
    icon: <ShareNetworkIcon />,
    items: [
      item('go-live', 'Go live', '/apps/$name/go-live', <RocketLaunchIcon />),
      item('domains', 'Domains', '/apps/$name/domains', <GlobeIcon />),
      item('streams', 'Streams', '/apps/$name/streams', <PlugsIcon />),
      item(
        'loadbalancer',
        'Load balancer',
        '/apps/$name/loadbalancer',
        <ArrowsSplitIcon />,
        'load-balancer',
      ),
      item('network', 'Network', '/apps/$name/network', <ShareNetworkIcon />),
      item('cache', 'Cache', '/apps/$name/cache', <LightningIcon />),
      {
        ...item('forms', 'Forms', '/apps/$name/forms', <ClipboardTextIcon />),
        staticOnly: true,
        beta: true,
      },
      {
        ...item(
          'static-routes',
          'Routing rules',
          '/apps/$name/static-routes',
          <SignpostIcon />,
        ),
        staticOnly: true,
      },
    ],
  },
  {
    id: 'config',
    label: 'Config',
    icon: <WrenchIcon />,
    items: [
      item(
        'environment',
        'Environment',
        '/apps/$name/environment',
        <BracketsCurlyIcon />,
      ),
      item('services', 'Services', '/apps/$name/services', <StackIcon />),
      item('resources', 'Resources', '/apps/$name/resources', <CpuIcon />),
      item('volumes', 'Volumes', '/apps/$name/volumes', <HardDrivesIcon />),
      item('health', 'Health', '/apps/$name/health', <HeartbeatIcon />),
      item(
        'scheduled-tasks',
        'Jobs',
        '/apps/$name/scheduled-tasks',
        <ClockCountdownIcon />,
      ),
      item(
        'feature-flags',
        'Feature flags',
        '/apps/$name/feature-flags',
        <FlagIcon />,
      ),
      item(
        'split-testing',
        'Split testing',
        '/apps/$name/split-testing',
        <FlaskIcon />,
      ),
      item(
        'integrations',
        'Integrations',
        '/apps/$name/integrations',
        <PlugsConnectedIcon />,
      ),
    ],
  },
  {
    id: 'observe',
    label: 'Observe',
    icon: <EyeIcon />,
    items: [
      item('metrics', 'Metrics', '/apps/$name/metrics', <PulseIcon />),
      item('logs', 'Logs', '/apps/$name/logs', <ScrollIcon />),
      item(
        'log-pipeline',
        'Log pipeline',
        '/apps/$name/log-pipeline',
        <FunnelIcon />,
      ),
      item(
        'requests',
        'Requests',
        '/apps/$name/requests',
        <ListMagnifyingGlassIcon />,
      ),
      item('traffic', 'Traffic', '/apps/$name/traffic', <ChartLineUpIcon />),
      item('traces', 'Traces', '/apps/$name/traces', <TreeStructureIcon />),
      item('errors', 'Errors', '/apps/$name/errors', <BugIcon />),
      item('alerts', 'Alerts', '/apps/$name/alerts', <BellIcon />),
    ],
  },
  {
    id: 'exec',
    label: 'Exec',
    icon: <TerminalIcon />,
    items: [item('exec', 'Exec', '/apps/$name/exec', <TerminalIcon />)],
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

// Every group starts open: a collapsed default hides most of the product on a
// first visit. Anything the user collapses is persisted and wins from then on.
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
