import type * as React from 'react'
import {
  ArrowsSplitIcon,
  BellIcon,
  BookOpenIcon,
  BracketsCurlyIcon,
  ClockCountdownIcon,
  CloudArrowUpIcon,
  CloudIcon,
  CpuIcon,
  DatabaseIcon,
  FlagIcon,
  FolderIcon,
  FolderOpenIcon,
  GaugeIcon,
  GavelIcon,
  GearIcon,
  GitBranchIcon,
  GlobeIcon,
  ListBulletsIcon,
  HardDrivesIcon,
  HeartbeatIcon,
  KeyIcon,
  PackageIcon,
  PlugsConnectedIcon,
  PlugsIcon,
  PulseIcon,
  RobotIcon,
  RocketLaunchIcon,
  ScrollIcon,
  ShareNetworkIcon,
  ShieldCheckIcon,
  ShieldWarningIcon,
  SlidersHorizontalIcon,
  SquaresFourIcon,
  StackIcon,
  TerminalIcon,
  TreeStructureIcon,
  ClockCounterClockwiseIcon,
  EyeIcon,
  TrafficSignalIcon,
  WrenchIcon,
} from '@phosphor-icons/react/dist/ssr'
import { GO_TARGETS } from '@/lib/shortcuts'
import { filterByFeature, type ExperimentalFeature } from '@/lib/experimental'

export type GlobalTo =
  | '/'
  | '/status'
  | '/apps'
  | '/deployments'
  | '/projects'
  | '/databases'
  | '/backups'
  | '/nodes'
  | '/network'
  | '/network/proxy'
  | '/domains'
  | '/dns'
  | '/loadbalancers'
  | '/models'
  | '/ai-assistant'
  | '/pipelines'
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
  | 'domains-attention'
  | 'dns-attention'
  | 'proxy-attention'

export interface GlobalNavItem {
  id: string
  label: string
  to: GlobalTo | PendingTo
  icon: React.ReactNode
  exact?: boolean
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
      },
      {
        id: 'projects',
        label: 'Projects',
        to: '/projects',
        icon: <FolderIcon />,
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
    ],
  },
  {
    id: 'traffic',
    label: 'Traffic',
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
        id: 'loadbalancers',
        label: 'Load balancers',
        to: '/loadbalancers',
        icon: <ArrowsSplitIcon />,
        feature: 'load-balancer',
      },
    ],
  },
  {
    id: 'infrastructure',
    label: 'Infrastructure',
    items: [
      { id: 'nodes', label: 'Nodes', to: '/nodes', icon: <HardDrivesIcon /> },
      {
        id: 'network',
        label: 'Network',
        to: '/network',
        icon: <ShareNetworkIcon />,
      },
      {
        id: 'registry',
        label: 'Container registry',
        to: '/settings/registry',
        icon: <PackageIcon />,
      },
      {
        id: 'registry-credentials',
        label: 'Registry credentials',
        to: '/settings/registry-credentials',
        icon: <KeyIcon />,
      },
      {
        id: 'network-shares',
        label: 'Network shares',
        to: '/settings/network-shares',
        icon: <FolderOpenIcon />,
      },
      {
        id: 'node-providers',
        label: 'Cloud node providers',
        to: '/settings/node-providers',
        icon: <CloudIcon />,
      },
    ],
  },
  {
    id: 'access',
    label: 'Access control',
    items: [
      {
        id: 'iam-policies',
        label: 'IAM policies',
        to: '/settings/iam-policies',
        icon: <ShieldCheckIcon />,
      },
      {
        id: 'security',
        label: 'Security center',
        to: '/security',
        icon: <ShieldWarningIcon />,
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
      { id: 'alerts', label: 'Alerts', to: '/alerts', icon: <BellIcon /> },
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
  { id: 'settings', label: 'Settings', to: '/settings', icon: <GearIcon /> },
  { id: 'help', label: 'Help', to: '/help', icon: <BookOpenIcon /> },
]

export function visibleGlobalGroups(
  enabled: readonly string[],
  routeAvailable: (to: string) => boolean = () => true,
): GlobalNavGroup[] {
  return GLOBAL_NAV_GROUPS.map((g) => ({
    ...g,
    items: filterByFeature(g.items, enabled).filter(
      (i) => !i.requiresRoute || routeAvailable(i.to),
    ),
  })).filter((g) => g.items.length > 0)
}

export function visibleGlobalFooter(
  enabled: readonly string[],
): GlobalNavItem[] {
  return filterByFeature(GLOBAL_NAV_FOOTER, enabled)
}

export function visibleAppSections(
  enabled: readonly string[],
): AppNavSection[] {
  return APP_NAV_SECTIONS.map((s) => ({
    ...s,
    items: filterByFeature(s.items, enabled),
  })).filter((s) => s.items.length > 0)
}

export function allGlobalItems(): GlobalNavItem[] {
  return [...GLOBAL_NAV_GROUPS.flatMap((g) => g.items), ...GLOBAL_NAV_FOOTER]
}

export function isGlobalItemActive(
  pathname: string,
  item: GlobalNavItem,
): boolean {
  if (item.exact) return pathname === item.to
  return pathname === item.to || pathname.startsWith(`${item.to}/`)
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
  | '/apps/$name/deploy-settings'
  | '/apps/$name/pipelines'
  | '/apps/$name/domains'
  | '/apps/$name/streams'
  | '/apps/$name/loadbalancer'
  | '/apps/$name/network'
  | '/apps/$name/environment'
  | '/apps/$name/services'
  | '/apps/$name/resources'
  | '/apps/$name/volumes'
  | '/apps/$name/health'
  | '/apps/$name/scheduled-tasks'
  | '/apps/$name/feature-flags'
  | '/apps/$name/integrations'
  | '/apps/$name/metrics'
  | '/apps/$name/logs'
  | '/apps/$name/alerts'
  | '/apps/$name/exec'

export interface AppNavItem {
  id: string
  slug: string
  label: string
  to: AppTo
  icon: React.ReactNode
  feature?: ExperimentalFeature
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
        'Scheduled tasks',
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

export function resolveOpen(
  stored: Record<string, boolean>,
  id: string,
  fallback: boolean,
): boolean {
  return stored[id] ?? fallback
}
