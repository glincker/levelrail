import * as React from 'react'
import {
  ActivityGlassIcon,
  ArrowUpGlassIcon,
  BellGlassIcon,
  BinaryTreeGlassIcon,
  BotGlassIcon,
  ChartLineGlassIcon,
  CircleQuestionMarkGlassIcon,
  CloudCheckGlassIcon,
  CoinsGlassIcon,
  ContrastGlassIcon,
  CpuGlassIcon,
  DatabaseGlassIcon,
  FolderGlassIcon,
  GaugeGlassIcon,
  GavelGlassIcon,
  GitBranchGlassIcon,
  GlobeGlassIcon,
  HammerGlassIcon,
  ImagesGlassIcon,
  KeyGlassIcon,
  LayersGlassIcon,
  LayoutGridGlassIcon,
  LockGlassIcon,
  MailGlassIcon,
  PackageGlassIcon,
  PlugGlassIcon,
  PlusGlassIcon,
  RestoreGlassIcon,
  RocketGlassIcon,
  ServerGlassIcon,
  SettingsGlassIcon,
  ShieldCheckGlassIcon,
  ShieldGlassIcon,
  SocialGithubGlassIcon,
  SplitGlassIcon,
  TerminalGlassIcon,
  UniversityGlassIcon,
  UploadGlassIcon,
  UserGlassIcon,
  UsersGlassIcon,
  WebhookGlassIcon,
  ZapGlassIcon,
} from '@thesvg/ui-react'
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
  /** Show the subtitle on its own line under the label, for long text. */
  stacked?: boolean
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
  kind: 'page' | 'app' | 'template' | 'action' | 'doc' | 'image' | 'assistant'
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
  hash?: string
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
  hash?: string,
): RouteEntry => ({ key, label, group, icon, to, feature, hash })

export const ROUTE_ENTRIES: RouteEntry[] = [
  nav(
    'action-status',
    'Go to Status',
    <ActivityGlassIcon size="1.4em" />,
    '/status',
    'Actions',
  ),
  nav(
    'action-apps',
    'Go to Apps',
    <LayersGlassIcon size="1.4em" />,
    '/apps',
    'Actions',
  ),
  nav(
    'action-alerts',
    'Go to Alerts',
    <BellGlassIcon size="1.4em" />,
    '/alerts',
    'Actions',
  ),
  nav(
    'action-nodes',
    'Go to Nodes',
    <ServerGlassIcon size="1.4em" />,
    '/nodes',
    'Actions',
  ),
  nav(
    'action-create-app',
    'Create app',
    <PlusGlassIcon size="1.4em" />,
    '/apps',
    'Actions',
  ),
  nav(
    'action-templates',
    'Browse templates',
    <LayoutGridGlassIcon size="1.4em" />,
    '/apps',
    'Actions',
  ),
  nav(
    'action-dashboard',
    'Go to dashboard',
    <GaugeGlassIcon size="1.4em" />,
    '/',
    'Actions',
  ),
  nav(
    'action-new-app',
    'New app',
    <PlusGlassIcon size="1.4em" />,
    '/apps',
    'Actions',
  ),
  nav(
    'action-import',
    'Import',
    <UploadGlassIcon size="1.4em" />,
    '/settings/import-platform',
    'Actions',
  ),
  {
    ...nav(
      'action-filter-failing',
      'Filter failing apps',
      <ActivityGlassIcon size="1.4em" />,
      '/apps',
      'Actions',
    ),
    search: { status: 'failing' },
  },
  nav('nav-dashboard', 'Dashboard', <GaugeGlassIcon size="1.4em" />, '/'),
  nav(
    'action-create-database',
    'Create database',
    <PlusGlassIcon size="1.4em" />,
    '/databases',
    'Actions',
  ),
  nav(
    'nav-databases',
    'Databases',
    <DatabaseGlassIcon size="1.4em" />,
    '/databases',
  ),
  nav(
    'nav-projects',
    'Projects',
    <FolderGlassIcon size="1.4em" />,
    '/projects',
  ),
  nav(
    'nav-pipelines',
    'Pipelines',
    <BinaryTreeGlassIcon size="1.4em" />,
    '/pipelines',
  ),
  nav('nav-domains', 'Domains', <GlobeGlassIcon size="1.4em" />, '/domains'),
  nav('nav-dns', 'DNS zones', <GlobeGlassIcon size="1.4em" />, '/dns'),
  nav(
    'nav-private-networks',
    'Private networks',
    <LockGlassIcon size="1.4em" />,
    '/network/private',
  ),
  nav(
    'action-private-networks',
    'Create a private network',
    <PlusGlassIcon size="1.4em" />,
    '/network/private',
    'Actions',
  ),
  nav(
    'nav-firewall',
    'Firewall',
    <ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />,
    '/firewall',
  ),
  nav(
    'nav-loadbalancers',
    'Load balancers',
    <SplitGlassIcon size="1.4em" />,
    '/loadbalancers',
    'Navigate',
    'load-balancer',
  ),
  nav(
    'nav-mail',
    'Mail',
    <MailGlassIcon size="1.4em" />,
    '/mail',
    'Navigate',
    'mail',
  ),
  nav('nav-backups', 'Backups', <UploadGlassIcon size="1.4em" />, '/backups'),
  nav('nav-assets', 'Assets', <ImagesGlassIcon size="1.4em" />, '/assets'),
  nav(
    'nav-deployments',
    'Deployments',
    <RocketGlassIcon size="1.4em" />,
    '/deployments',
  ),
  nav(
    'nav-deploy-timeline',
    'Deploy timeline',
    <RestoreGlassIcon size="1.4em" />,
    '/deploy-timeline',
  ),
  nav('nav-usage', 'Usage and cost', <CoinsGlassIcon size="1.4em" />, '/usage'),
  nav(
    'nav-costs',
    'Cost explorer',
    <ChartLineGlassIcon size="1.4em" />,
    '/costs',
  ),
  nav('nav-builds', 'Builds', <HammerGlassIcon size="1.4em" />, '/builds'),
  nav(
    'nav-functions',
    'Functions',
    <ZapGlassIcon size="1.4em" />,
    '/functions',
  ),
  nav(
    'nav-approvals',
    'Deploy approvals',
    <GavelGlassIcon size="1.4em" />,
    '/approvals',
  ),
  nav(
    'nav-models',
    'AI models',
    <CpuGlassIcon size="1.4em" />,
    '/models',
    'Navigate',
    'ai-models',
  ),
  nav(
    'nav-assistant',
    'AI assistant',
    <BotGlassIcon size="1.4em" />,
    '/ai-assistant',
    'Navigate',
    'ai-chat',
  ),
  nav(
    'nav-console',
    'Console',
    <TerminalGlassIcon size="1.4em" />,
    '/terminal',
    'Navigate',
    'cli-console',
  ),
  nav(
    'nav-help',
    'Help',
    <CircleQuestionMarkGlassIcon size="1.4em" />,
    '/help',
  ),
  nav(
    'settings-hub',
    'Settings',
    <SettingsGlassIcon size="1.4em" />,
    '/settings',
    'Settings',
  ),
  nav(
    'settings-platform',
    'Platform settings',
    <SettingsGlassIcon size="1.4em" />,
    '/settings/platform',
    'Settings',
  ),
  nav(
    'settings-platform-https',
    'HTTPS settings',
    <LockGlassIcon size="1.4em" />,
    '/settings/platform/https',
    'Settings',
  ),
  nav(
    'settings-platform-ingress',
    'Ingress',
    <PlugGlassIcon size="1.4em" />,
    '/settings/platform/ingress',
    'Settings',
  ),
  nav(
    'settings-platform-addresses',
    'Addresses',
    <GlobeGlassIcon size="1.4em" />,
    '/settings/platform/addresses',
    'Settings',
  ),
  nav(
    'settings-platform-dns-providers',
    'DNS providers',
    <CloudCheckGlassIcon size="1.4em" />,
    '/settings/platform/dns-providers',
    'Settings',
  ),
  nav(
    'settings-platform-enable-https',
    'Enable HTTPS',
    <LockGlassIcon size="1.4em" />,
    '/settings/platform/https',
    'Settings',
    undefined,
    'enable-https',
  ),
  nav(
    'settings-platform-dashboard-address',
    'Dashboard address',
    <GlobeGlassIcon size="1.4em" />,
    '/settings/platform/addresses',
    'Settings',
    undefined,
    'dashboard-url',
  ),
  nav(
    'settings-platform-apps-base-domain',
    'Apps base domain',
    <GlobeGlassIcon size="1.4em" />,
    '/settings/platform/addresses',
    'Settings',
    undefined,
    'apps-base-domain',
  ),
  nav(
    'settings-platform-fallback-domains',
    'Fallback domains',
    <GlobeGlassIcon size="1.4em" />,
    '/settings/platform/addresses',
    'Settings',
    undefined,
    'fallback-domains',
  ),
  nav(
    'settings-platform-ingress-settings',
    'Ingress settings',
    <PlugGlassIcon size="1.4em" />,
    '/settings/platform/ingress',
    'Settings',
    undefined,
    'ingress-settings',
  ),
  nav(
    'settings-platform-reverse-proxy',
    'Reverse proxy guide',
    <PlugGlassIcon size="1.4em" />,
    '/settings/platform/ingress',
    'Settings',
    undefined,
    'reverse-proxy-guide',
  ),
  nav(
    'settings-platform-cloudflare-dns',
    'Cloudflare DNS',
    <CloudCheckGlassIcon size="1.4em" />,
    '/settings/platform/dns-providers',
    'Settings',
    undefined,
    'cloudflare-dns',
  ),
  nav(
    'settings-platform-route53',
    'Route 53',
    <CloudCheckGlassIcon size="1.4em" />,
    '/settings/platform/dns-providers',
    'Settings',
    undefined,
    'route53-dns',
  ),
  nav(
    'settings-platform-google-cloud-dns',
    'Google Cloud DNS',
    <CloudCheckGlassIcon size="1.4em" />,
    '/settings/platform/dns-providers',
    'Settings',
    undefined,
    'google-cloud-dns',
  ),
  nav(
    'settings-account',
    'Account',
    <UserGlassIcon size="1.4em" />,
    '/settings/account',
    'Settings',
  ),
  nav(
    'settings-security',
    'Security',
    <ShieldGlassIcon size="1.4em" />,
    '/settings/security',
    'Settings',
  ),
  nav(
    'settings-tokens',
    'API tokens',
    <KeyGlassIcon size="1.4em" />,
    '/settings/tokens',
    'Settings',
  ),
  nav(
    'settings-agents',
    'Agents',
    <PlugGlassIcon size="1.4em" />,
    '/settings/agents',
    'Settings',
  ),
  nav(
    'settings-backup-targets',
    'Backup targets',
    <UploadGlassIcon size="1.4em" />,
    '/settings/backup-targets',
    'Settings',
  ),
  nav(
    'settings-storage',
    'Storage destinations',
    <UploadGlassIcon size="1.4em" />,
    '/settings/storage',
    'Settings',
  ),
  nav(
    'settings-git-connections',
    'Git connections',
    <GitBranchGlassIcon size="1.4em" />,
    '/settings/git-connections',
    'Settings',
  ),
  nav(
    'settings-registry-credentials',
    'Registry credentials',
    <PackageGlassIcon size="1.4em" />,
    '/settings/registry-credentials',
    'Settings',
  ),
  nav(
    'settings-status-page',
    'Status page',
    <ActivityGlassIcon size="1.4em" />,
    '/settings/status-page',
    'Settings',
  ),
  nav(
    'settings-notification-channels',
    'Notification channels',
    <WebhookGlassIcon size="1.4em" />,
    '/settings/notification-channels',
    'Settings',
  ),
  nav(
    'settings-github-app',
    'GitHub App',
    <SocialGithubGlassIcon size="1.4em" />,
    '/settings/github-app',
    'Settings',
  ),
  nav(
    'settings-oauth',
    'OAuth sign-in',
    <KeyGlassIcon size="1.4em" />,
    '/settings/oauth',
    'Settings',
  ),
  nav(
    'settings-organizations',
    'Organizations',
    <UniversityGlassIcon size="1.4em" />,
    '/settings/organizations',
    'Settings',
  ),
  nav(
    'settings-users',
    'Users',
    <UsersGlassIcon size="1.4em" />,
    '/settings/users',
    'Settings',
  ),
  nav(
    'settings-email',
    'Email',
    <MailGlassIcon size="1.4em" />,
    '/settings/email',
    'Settings',
  ),
  nav(
    'settings-general',
    'General',
    <SettingsGlassIcon size="1.4em" />,
    '/settings/general',
    'Settings',
  ),
  nav(
    'settings-updates',
    'Updates',
    <ArrowUpGlassIcon size="1.4em" />,
    '/settings/updates',
    'Settings',
  ),
  nav(
    'settings-audit-log',
    'Audit log',
    <RestoreGlassIcon size="1.4em" />,
    '/settings/audit-log',
    'Settings',
  ),
  nav(
    'settings-dr-report',
    'Recovery report',
    <ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />,
    '/settings/dr-report',
    'Settings',
  ),
  nav(
    'settings-heartbeat',
    'Heartbeat',
    <ActivityGlassIcon size="1.4em" />,
    '/settings/notification-channels',
    'Settings',
  ),
  nav(
    'settings-deploy-policies',
    'Deploy policies and CI gate',
    <GavelGlassIcon size="1.4em" />,
    '/settings/deploy-policies',
    'Settings',
  ),
  nav(
    'settings-cloudflare-tunnel',
    'Cloudflare Tunnel',
    <GlobeGlassIcon size="1.4em" />,
    '/settings/cloudflare-tunnel',
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
  icon: <ContrastGlassIcon size="1.4em" />,
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
    hash?: string,
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
    run: go(e.to, undefined, e.search, e.hash),
    hint: NO_HINT_KEYS.has(e.key) ? undefined : chordFor(e.to),
  }
}
