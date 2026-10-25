import * as React from 'react'
import {
  GaugeIcon,
  GitBranchIcon,
  LockKeyIcon,
  StackIcon,
  DatabaseIcon,
  FolderIcon,
  BuildingsIcon,
  HardDrivesIcon,
  GlobeHemisphereWestIcon,
  GlobeIcon,
  UserIcon,
  ShieldIcon,
  ShieldCheckIcon,
  KeyIcon,
  PlugsConnectedIcon,
  CloudArrowUpIcon,
  WebhooksLogoIcon,
  GithubLogoIcon,
  UsersIcon,
  EnvelopeIcon,
  GearIcon,
  PackageIcon,
  QuestionIcon,
  ArrowCircleUpIcon,
  ClockCounterClockwiseIcon,
  HeartbeatIcon,
  PlusIcon,
  SquaresFourIcon,
  CircleHalfIcon,
  ArrowsSplitIcon,
  TreeStructureIcon,
  CpuIcon,
  GavelIcon,
  RocketLaunchIcon,
  HammerIcon,
  CoinsIcon,
  ChartLineUpIcon,
  RobotIcon,
  TerminalWindowIcon,
  BellIcon,
  LightningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  BETA_FEATURES_PATH,
  isFeatureLocked,
  lockedSearch,
  type ExperimentalFeature,
} from '../lib/experimental'
import type { SearchSpan } from '../queries/search'
import { chordFor } from './shell/navModel'

export interface PaletteItem {
  key: string
  label: string
  group: string
  icon: React.ReactNode
  run: () => void
  hint?: string[]
  /** Extra match text ranked below the label, e.g. a domain's app name. */
  keywords?: string
  /** Trailing status chip, e.g. a domain's status. */
  badge?: React.ReactNode
  /** Muted second line, e.g. why an entry is locked. */
  subtitle?: string
  /** True for a gated destination that is switched off. */
  locked?: boolean
  /** Label spans to highlight, computed by the server for search hits. */
  highlightSpans?: SearchSpan[]
  /** Run without closing the palette (expand a group, apply a chip). */
  keepOpen?: boolean
  /** Ask for a confirm step before run (destructive server actions). */
  confirm?: boolean
  /** Tab / Cmd+Enter action and the hint word shown for it. */
  secondary?: () => void
  secondaryHint?: string
  /** Trailing remove control, e.g. delete a saved search. */
  onRemove?: () => void
  removeLabel?: string
  /** Search hit ref, recorded as usage on select. */
  searchRef?: string
  /** Preview pane content: what this entry is and where it leads. */
  preview?: PalettePreviewData
}

export interface PalettePreviewData {
  kind: 'page' | 'app' | 'template' | 'action'
  description?: string
  path?: string
  /** Label of the primary (Enter) button. */
  primaryLabel?: string
  /** Small facts shown as chips: category, status, requirements. */
  facts?: string[]
}

export interface RouteEntry {
  key: string
  label: string
  group: string
  icon: React.ReactNode
  to: string
  search?: Record<string, string>
  feature?: ExperimentalFeature
}

export const GROUP_ORDER = [
  'Suggested',
  'Recent',
  'App actions',
  'Domain actions',
  'Navigate',
  'Settings',
  'Apps',
  'Databases',
  'Nodes',
  'Buckets',
  'Private networks',
  'Templates',
  'Tags',
  'Saved views',
  'Domains',
  'DNS zones',
  'DNS records',
  'Actions',
] as const

const nav = (
  key: string,
  label: string,
  icon: React.ReactNode,
  to: string,
  group = 'Navigate',
  feature?: ExperimentalFeature,
): RouteEntry => ({ key, label, group, icon, to, feature })

export const ROUTE_ENTRIES: RouteEntry[] = [
  nav('action-status', 'Go to Status', <HeartbeatIcon />, '/status', 'Actions'),
  nav('action-apps', 'Go to Apps', <StackIcon />, '/apps', 'Actions'),
  nav('action-alerts', 'Go to Alerts', <BellIcon />, '/alerts', 'Actions'),
  nav('action-nodes', 'Go to Nodes', <HardDrivesIcon />, '/nodes', 'Actions'),
  nav('action-create-app', 'Create app', <PlusIcon />, '/apps', 'Actions'),
  nav(
    'action-templates',
    'Browse templates',
    <SquaresFourIcon />,
    '/apps',
    'Actions',
  ),
  nav('action-dashboard', 'Go to dashboard', <GaugeIcon />, '/', 'Actions'),
  nav('action-new-app', 'New app', <PlusIcon />, '/apps', 'Actions'),
  nav(
    'action-import',
    'Import',
    <CloudArrowUpIcon />,
    '/settings/import-platform',
    'Actions',
  ),
  {
    ...nav(
      'action-filter-failing',
      'Filter failing apps',
      <HeartbeatIcon />,
      '/apps',
      'Actions',
    ),
    search: { status: 'failing' },
  },
  nav('nav-dashboard', 'Dashboard', <GaugeIcon />, '/'),
  nav(
    'action-create-database',
    'Create database',
    <PlusIcon />,
    '/databases',
    'Actions',
  ),
  nav('nav-databases', 'Databases', <DatabaseIcon />, '/databases'),
  nav('nav-projects', 'Projects', <FolderIcon />, '/projects'),
  nav('nav-pipelines', 'Pipelines', <TreeStructureIcon />, '/pipelines'),
  nav('nav-domains', 'Domains', <GlobeIcon />, '/domains'),
  nav('nav-dns', 'DNS zones', <GlobeHemisphereWestIcon />, '/dns'),
  nav(
    'nav-private-networks',
    'Private networks',
    <LockKeyIcon />,
    '/network/private',
  ),
  nav(
    'action-private-networks',
    'Create a private network',
    <PlusIcon />,
    '/network/private',
    'Actions',
  ),
  nav('nav-firewall', 'Firewall', <ShieldCheckIcon />, '/firewall'),
  nav(
    'nav-loadbalancers',
    'Load balancers',
    <ArrowsSplitIcon />,
    '/loadbalancers',
    'Navigate',
    'load-balancer',
  ),
  nav('nav-backups', 'Backups', <CloudArrowUpIcon />, '/backups'),
  nav('nav-deployments', 'Deployments', <RocketLaunchIcon />, '/deployments'),
  nav(
    'nav-deploy-timeline',
    'Deploy timeline',
    <ClockCounterClockwiseIcon />,
    '/deploy-timeline',
  ),
  nav('nav-usage', 'Usage and cost', <CoinsIcon />, '/usage'),
  nav('nav-costs', 'Cost explorer', <ChartLineUpIcon />, '/costs'),
  nav('nav-builds', 'Builds', <HammerIcon />, '/builds'),
  nav('nav-functions', 'Functions', <LightningIcon />, '/functions'),
  nav('nav-approvals', 'Deploy approvals', <GavelIcon />, '/approvals'),
  nav(
    'nav-models',
    'AI models',
    <CpuIcon />,
    '/models',
    'Navigate',
    'ai-models',
  ),
  nav(
    'nav-assistant',
    'AI assistant',
    <RobotIcon />,
    '/ai-assistant',
    'Navigate',
    'ai-chat',
  ),
  nav(
    'nav-console',
    'Console',
    <TerminalWindowIcon />,
    '/terminal',
    'Navigate',
    'cli-console',
  ),
  nav('nav-help', 'Help', <QuestionIcon />, '/help'),
  nav('settings-hub', 'Settings', <GearIcon />, '/settings', 'Settings'),
  nav(
    'settings-account',
    'Account',
    <UserIcon />,
    '/settings/account',
    'Settings',
  ),
  nav(
    'settings-security',
    'Security',
    <ShieldIcon />,
    '/settings/security',
    'Settings',
  ),
  nav(
    'settings-tokens',
    'API tokens',
    <KeyIcon />,
    '/settings/tokens',
    'Settings',
  ),
  nav(
    'settings-agents',
    'Agents',
    <PlugsConnectedIcon />,
    '/settings/agents',
    'Settings',
  ),
  nav(
    'settings-backup-targets',
    'Backup targets',
    <CloudArrowUpIcon />,
    '/settings/backup-targets',
    'Settings',
  ),
  nav(
    'settings-storage',
    'Storage destinations',
    <CloudArrowUpIcon />,
    '/settings/storage',
    'Settings',
  ),
  nav(
    'settings-git-connections',
    'Git connections',
    <GitBranchIcon />,
    '/settings/git-connections',
    'Settings',
  ),
  nav(
    'settings-registry-credentials',
    'Registry credentials',
    <PackageIcon />,
    '/settings/registry-credentials',
    'Settings',
  ),
  nav(
    'settings-status-page',
    'Status page',
    <HeartbeatIcon />,
    '/settings/status-page',
    'Settings',
  ),
  nav(
    'settings-notification-channels',
    'Notification channels',
    <WebhooksLogoIcon />,
    '/settings/notification-channels',
    'Settings',
  ),
  nav(
    'settings-github-app',
    'GitHub App',
    <GithubLogoIcon />,
    '/settings/github-app',
    'Settings',
  ),
  nav(
    'settings-oauth',
    'OAuth sign-in',
    <KeyIcon />,
    '/settings/oauth',
    'Settings',
  ),
  nav(
    'settings-organizations',
    'Organizations',
    <BuildingsIcon />,
    '/settings/organizations',
    'Settings',
  ),
  nav('settings-users', 'Users', <UsersIcon />, '/settings/users', 'Settings'),
  nav(
    'settings-email',
    'Email',
    <EnvelopeIcon />,
    '/settings/email',
    'Settings',
  ),
  nav(
    'settings-general',
    'General',
    <GearIcon />,
    '/settings/general',
    'Settings',
  ),
  nav(
    'settings-updates',
    'Updates',
    <ArrowCircleUpIcon />,
    '/settings/updates',
    'Settings',
  ),
  nav(
    'settings-audit-log',
    'Audit log',
    <ClockCounterClockwiseIcon />,
    '/settings/audit-log',
    'Settings',
  ),
]

// Always-available empty-query CTAs, independent of recents/app data.
// Keys into ROUTE_ENTRIES/baseItems; order is the display order.
export const DEFAULT_QUICK_ACTION_KEYS = [
  'action-create-app',
  'action-apps',
  'nav-deployments',
  'nav-help',
  'action-shortcuts',
] as const

export const THEME_ACTION = {
  key: 'action-toggle-theme',
  label: 'Toggle theme',
  group: 'Actions',
  icon: <CircleHalfIcon />,
} as const

const NO_HINT_KEYS = new Set(['action-create-app', 'action-templates'])

// A gated destination that is switched off stays searchable and opens Beta
// features for that feature instead of its own (guarded) page.
export function routeEntryItem(
  e: RouteEntry,
  enabled: readonly string[],
  go: (
    to: string,
    params?: Record<string, string>,
    search?: Record<string, string>,
  ) => () => void,
  lockedSubtitle: string,
): PaletteItem {
  const base = {
    key: e.key,
    label: e.label,
    group: e.group,
    icon: e.icon,
    preview: { kind: 'page' as const, path: e.to },
  }
  if (isFeatureLocked(e.feature, enabled)) {
    return {
      ...base,
      run: go(BETA_FEATURES_PATH, undefined, { ...lockedSearch(e.feature) }),
      subtitle: lockedSubtitle,
      keywords: 'beta locked',
      locked: true,
    }
  }
  return {
    ...base,
    run: go(e.to, undefined, e.search),
    hint: NO_HINT_KEYS.has(e.key) ? undefined : chordFor(e.to),
  }
}
