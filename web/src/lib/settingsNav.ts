import type { ComponentType } from 'react'
import {
  ArrowCircleUpIcon,
  BroomIcon,
  BuildingsIcon,
  StackSimpleIcon,
  SquaresFourIcon,
  ChartBarIcon,
  ChartLineIcon,
  ClockCounterClockwiseIcon,
  SlidersHorizontalIcon,
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
  GaugeIcon,
  GearIcon,
  GitBranchIcon,
  GitForkIcon,
  GithubLogoIcon,
  GitlabLogoIcon,
  GlobeIcon,
  HeartbeatIcon,
  WrenchIcon,
  IdentificationBadgeIcon,
  IdentificationCardIcon,
  KeyIcon,
  LockKeyIcon,
  PlugsConnectedIcon,
  RobotIcon,
  ShareNetworkIcon,
  ShieldCheckIcon,
  ShieldIcon,
  StackIcon,
  TeaBagIcon,
  TerminalWindowIcon,
  TimerIcon,
  UserIcon,
  UsersIcon,
  UsersThreeIcon,
  WebhooksLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { ExperimentalFeature } from './experimental'

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
      {
        id: 'account',
        to: '/settings/account',
        icon: UserIcon,
      },
      {
        id: 'general',
        to: '/settings/general',
        icon: GearIcon,
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
      {
        id: 'stacks',
        to: '/settings/stacks',
        icon: StackSimpleIcon,
        feature: 'iac',
      },
      {
        id: 'stack-templates',
        to: '/settings/stack-templates',
        icon: SquaresFourIcon,
        feature: 'iac',
      },
      {
        id: 'import-platform',
        to: '/settings/import-platform',
        icon: DownloadSimpleIcon,
      },
      {
        id: 'data-export',
        to: '/settings/data-export',
        icon: ExportIcon,
      },
    ],
  },
  {
    id: 'access',
    items: [
      {
        id: 'users',
        to: '/settings/users',
        icon: UsersIcon,
      },
      {
        id: 'teams',
        to: '/settings/teams',
        icon: UsersThreeIcon,
      },
      {
        id: 'roles',
        to: '/settings/roles',
        icon: IdentificationBadgeIcon,
        feature: 'access-roles',
      },
      {
        id: 'sso',
        to: '/settings/sso',
        icon: IdentificationCardIcon,
        feature: 'saml-scim',
      },
      {
        id: 'session-policy',
        to: '/settings/session-policy',
        icon: TimerIcon,
      },
      {
        id: 'oauth',
        to: '/settings/oauth',
        icon: LockKeyIcon,
      },
      {
        id: 'tokens',
        to: '/settings/tokens',
        icon: KeyIcon,
      },
      {
        id: 'mobile-devices',
        to: '/settings/mobile-devices',
        icon: DeviceMobileIcon,
        feature: 'mobile-api',
      },
      {
        id: 'plugins',
        to: '/settings/plugins',
        icon: PlugsConnectedIcon,
        feature: 'plugins',
      },
      {
        id: 'audit-log',
        to: '/settings/audit-log',
        icon: ClockCounterClockwiseIcon,
      },
    ],
  },
  {
    id: 'security',
    items: [
      {
        id: 'security',
        to: '/settings/security',
        icon: ShieldIcon,
      },
      {
        id: 'security-center',
        to: '/security',
        icon: ShieldCheckIcon,
      },
      {
        id: 'security-runtime',
        to: '/settings/security-runtime',
        icon: SlidersHorizontalIcon,
      },
      {
        id: 'vault',
        to: '/settings/vault',
        icon: LockKeyIcon,
      },
      {
        id: 'firewall',
        to: '/settings/firewall',
        icon: ShieldIcon,
      },
    ],
  },
  {
    id: 'developer',
    items: [
      {
        id: 'cli-access',
        to: '/settings/cli-access',
        icon: TerminalWindowIcon,
      },
      {
        id: 'api-explorer',
        to: '/settings/api-explorer',
        icon: CodeIcon,
      },
    ],
  },
  {
    id: 'ai',
    items: [
      {
        id: 'agents',
        to: '/settings/agents',
        icon: PlugsConnectedIcon,
      },
      {
        id: 'ai-control',
        to: '/settings/ai-control',
        icon: ShieldCheckIcon,
      },
      {
        id: 'ai-assistant',
        to: '/settings/ai-assistant',
        icon: RobotIcon,
        feature: 'ai-chat',
      },
    ],
  },
  {
    id: 'domains',
    items: [
      {
        id: 'domains',
        to: '/domains',
        icon: GlobeIcon,
      },
      {
        id: 'certificates',
        to: '/settings/certificates',
        icon: ShieldCheckIcon,
      },
      {
        id: 'cloudflare-tunnel',
        to: '/settings/cloudflare-tunnel',
        icon: CloudCheckIcon,
        feature: 'cloudflare-tunnel',
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
      {
        id: 'storage',
        to: '/settings/storage',
        icon: CloudArrowUpIcon,
      },
      {
        id: 'control-plane-backup',
        to: '/settings/control-plane-backup',
        icon: DatabaseIcon,
      },
      {
        id: 'database-upgrades',
        to: '/settings/database-upgrades',
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
      {
        id: 'event-webhooks',
        to: '/settings/event-webhooks',
        icon: WebhooksLogoIcon,
      },
      {
        id: 'traffic-export',
        to: '/settings/traffic-export',
        icon: ShareNetworkIcon,
      },
      {
        id: 'email',
        to: '/settings/email',
        icon: EnvelopeIcon,
      },
      {
        id: 'email-templates',
        to: '/settings/email-templates',
        icon: EnvelopeIcon,
      },
      {
        id: 'email-audiences',
        to: '/settings/email-audiences',
        icon: UsersThreeIcon,
      },
      {
        id: 'email-messages',
        to: '/settings/email-messages',
        icon: EnvelopeIcon,
      },
      {
        id: 'status-page',
        to: '/settings/status-page',
        icon: HeartbeatIcon,
      },
      {
        id: 'maintenance',
        to: '/settings/maintenance',
        icon: WrenchIcon,
      },
      {
        id: 'deploy-policies',
        to: '/settings/deploy-policies',
        icon: ShieldCheckIcon,
      },
      {
        id: 'build-performance',
        to: '/settings/build-performance',
        icon: TimerIcon,
      },
    ],
  },
  {
    id: 'integrations',
    items: [
      {
        id: 'git-connections',
        to: '/settings/git-connections',
        icon: GitBranchIcon,
      },
      {
        id: 'github-app',
        to: '/settings/github-app',
        icon: GithubLogoIcon,
      },
      {
        id: 'gitlab-app',
        to: '/settings/gitlab-app',
        icon: GitlabLogoIcon,
      },
      {
        id: 'bitbucket-app',
        to: '/settings/bitbucket-app',
        icon: GitBranchIcon,
      },
      {
        id: 'gitea-app',
        to: '/settings/gitea-app',
        icon: TeaBagIcon,
      },
      {
        id: 'forgejo-app',
        to: '/settings/forgejo-app',
        icon: GitForkIcon,
      },
      {
        id: 'bitbucket-server-app',
        to: '/settings/bitbucket-server-app',
        icon: GitBranchIcon,
      },
    ],
  },
  {
    id: 'beta',
    items: [
      {
        id: 'beta-features',
        to: '/settings/beta-features',
        icon: FlaskIcon,
      },
    ],
  },
  {
    id: 'updates',
    items: [
      {
        id: 'updates',
        to: '/settings/updates',
        icon: ArrowCircleUpIcon,
      },
    ],
  },
  {
    id: 'advanced',
    items: [
      {
        id: 'containers',
        to: '/settings/containers',
        icon: StackIcon,
      },
      {
        id: 'cleanup',
        to: '/settings/cleanup',
        icon: BroomIcon,
      },
      {
        id: 'infrastructure',
        to: '/settings/infrastructure',
        icon: FileCodeIcon,
        feature: 'iac',
      },
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
      {
        id: 'control-plane-health',
        to: '/settings/control-plane-health',
        icon: GaugeIcon,
      },
      {
        id: 'benchmarks',
        to: '/settings/benchmarks',
        icon: ChartBarIcon,
      },
    ],
  },
]

// Gated pages stay listed; useSettingsNav marks the switched-off ones locked.
export function visibleSettingsSections(): SettingsNavSection[] {
  return settingsNavSections
}
