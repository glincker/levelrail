import type { ComponentType } from 'react'
import {
  ArrowCircleUpIcon,
  BuildingsIcon,
  ChartLineIcon,
  ClockCounterClockwiseIcon,
  CloudArrowUpIcon,
  CloudCheckIcon,
  CodeIcon,
  DatabaseIcon,
  DeviceMobileIcon,
  DownloadSimpleIcon,
  EnvelopeIcon,
  ExportIcon,
  FileCodeIcon,
  FlaskIcon,
  GearIcon,
  GitBranchIcon,
  GithubLogoIcon,
  GitlabLogoIcon,
  GlobeIcon,
  HardDrivesIcon,
  HeartbeatIcon,
  IdentificationBadgeIcon,
  IdentificationCardIcon,
  KeyIcon,
  LockKeyIcon,
  PackageIcon,
  PlugsConnectedIcon,
  RobotIcon,
  ShieldCheckIcon,
  ShieldIcon,
  SparkleIcon,
  StackIcon,
  TeaBagIcon,
  TerminalWindowIcon,
  TimerIcon,
  UserIcon,
  UsersIcon,
  UsersThreeIcon,
  WebhooksLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
import { filterByFeature, type ExperimentalFeature } from './experimental'

import type settingsEn from '../locales/en/settings.json'

export type SettingsItemId = keyof (typeof settingsEn)['nav']['items']
export type SettingsAreaId = keyof (typeof settingsEn)['nav']['areas']

export interface SettingsNavItem {
  id: SettingsItemId
  to: string
  icon: ComponentType<{ className?: string }>
  feature?: ExperimentalFeature
}

export interface SettingsNavSection {
  id: SettingsAreaId
  items: SettingsNavItem[]
}

// Single source of truth for every settings page. Labels live in
// locales/en/settings.json under nav.areas.<id> and nav.items.<id>, read
// through useSettingsNav, so the hub and the sidebar never drift apart.
export const settingsNavSections: SettingsNavSection[] = [
  {
    id: 'general',
    items: [
      { id: 'account', to: '/settings/account', icon: UserIcon },
      {
        id: 'cli-access',
        to: '/settings/cli-access',
        icon: TerminalWindowIcon,
      },
      { id: 'api-explorer', to: '/settings/api-explorer', icon: CodeIcon },
      { id: 'data-export', to: '/settings/data-export', icon: ExportIcon },
      {
        id: 'import-platform',
        to: '/settings/import-platform',
        icon: DownloadSimpleIcon,
      },
      { id: 'setup-wizard', to: '/settings/setup-wizard', icon: SparkleIcon },
      { id: 'general', to: '/settings/general', icon: GearIcon },
    ],
  },
  {
    id: 'security',
    items: [
      {
        id: 'iam-policies',
        to: '/settings/iam-policies',
        icon: ShieldCheckIcon,
      },
      { id: 'security', to: '/settings/security', icon: ShieldIcon },
      { id: 'security-center', to: '/security', icon: ShieldCheckIcon },
      { id: 'tokens', to: '/settings/tokens', icon: KeyIcon },
      {
        id: 'mobile-devices',
        to: '/settings/mobile-devices',
        icon: DeviceMobileIcon,
        feature: 'mobile-api',
      },
      { id: 'agents', to: '/settings/agents', icon: PlugsConnectedIcon },
      { id: 'ai-control', to: '/settings/ai-control', icon: ShieldCheckIcon },
      { id: 'users', to: '/settings/users', icon: UsersIcon },
      { id: 'teams', to: '/settings/teams', icon: UsersThreeIcon },
      {
        id: 'roles',
        to: '/settings/roles',
        icon: IdentificationBadgeIcon,
        feature: 'access-roles',
      },
      { id: 'oauth', to: '/settings/oauth', icon: LockKeyIcon },
      { id: 'sso', to: '/settings/sso', icon: IdentificationCardIcon },
      {
        id: 'session-policy',
        to: '/settings/session-policy',
        icon: TimerIcon,
      },
      {
        id: 'organizations',
        to: '/settings/organizations',
        icon: BuildingsIcon,
      },
      {
        id: 'environments',
        to: '/settings/environments',
        icon: StackIcon,
        feature: 'global-environments',
      },
      { id: 'vault', to: '/settings/vault', icon: LockKeyIcon },
      {
        id: 'audit-log',
        to: '/settings/audit-log',
        icon: ClockCounterClockwiseIcon,
      },
    ],
  },
  {
    id: 'domains',
    items: [
      {
        id: 'cloudflare-tunnel',
        to: '/settings/cloudflare-tunnel',
        icon: CloudCheckIcon,
        feature: 'cloudflare-tunnel',
      },
      { id: 'domains', to: '/domains', icon: GlobeIcon },
      {
        id: 'certificates',
        to: '/settings/certificates',
        icon: ShieldCheckIcon,
      },
    ],
  },
  {
    id: 'backups',
    items: [
      {
        id: 'backup-targets',
        to: '/settings/backup-targets',
        icon: CloudArrowUpIcon,
      },
      { id: 'storage', to: '/settings/storage', icon: CloudArrowUpIcon },
      {
        id: 'control-plane-backup',
        to: '/settings/control-plane-backup',
        icon: DatabaseIcon,
      },
    ],
  },
  {
    id: 'notifications',
    items: [
      {
        id: 'notification-channels',
        to: '/settings/notification-channels',
        icon: WebhooksLogoIcon,
      },
      { id: 'status-page', to: '/settings/status-page', icon: HeartbeatIcon },
      { id: 'email', to: '/settings/email', icon: EnvelopeIcon },
    ],
  },
  {
    id: 'integrations',
    items: [
      { id: 'github-app', to: '/settings/github-app', icon: GithubLogoIcon },
      { id: 'gitlab-app', to: '/settings/gitlab-app', icon: GitlabLogoIcon },
      {
        id: 'bitbucket-app',
        to: '/settings/bitbucket-app',
        icon: GitBranchIcon,
      },
      { id: 'gitea-app', to: '/settings/gitea-app', icon: TeaBagIcon },
      {
        id: 'registry-credentials',
        to: '/settings/registry-credentials',
        icon: PackageIcon,
      },
      {
        id: 'node-providers',
        to: '/settings/node-providers',
        icon: HardDrivesIcon,
      },
      { id: 'registry', to: '/settings/registry', icon: HardDrivesIcon },
      {
        id: 'ai-assistant',
        to: '/settings/ai-assistant',
        icon: RobotIcon,
        feature: 'ai-chat',
      },
    ],
  },
  {
    id: 'beta',
    items: [
      { id: 'beta-features', to: '/settings/beta-features', icon: FlaskIcon },
    ],
  },
  {
    id: 'updates',
    items: [
      { id: 'updates', to: '/settings/updates', icon: ArrowCircleUpIcon },
    ],
  },
  {
    id: 'advanced',
    items: [
      {
        id: 'observability',
        to: '/settings/observability',
        icon: ChartLineIcon,
      },
      {
        id: 'system-status',
        to: '/settings/system-status',
        icon: HeartbeatIcon,
      },
      { id: 'firewall', to: '/settings/firewall', icon: ShieldIcon },
      { id: 'containers', to: '/settings/containers', icon: StackIcon },
      {
        id: 'infrastructure',
        to: '/settings/infrastructure',
        icon: FileCodeIcon,
        feature: 'iac',
      },
      {
        id: 'database-upgrades',
        to: '/settings/database-upgrades',
        icon: DatabaseIcon,
      },
    ],
  },
]

export function visibleSettingsSections(
  enabled: readonly string[],
): SettingsNavSection[] {
  return settingsNavSections
    .map((s) => ({ ...s, items: filterByFeature(s.items, enabled) }))
    .filter((s) => s.items.length > 0)
}
