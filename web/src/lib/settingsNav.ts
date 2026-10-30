import type { ComponentType } from 'react'
import {
  ActivityGlassIcon,
  ArrowUpGlassIcon,
  BotGlassIcon,
  BroomGlassIcon,
  ChartColumnGlassIcon,
  ChartLineGlassIcon,
  ClipboardListGlassIcon,
  ClockGlassIcon,
  CloudCheckGlassIcon,
  CodeGlassIcon,
  CoffeeGlassIcon,
  DatabaseGlassIcon,
  DownloadGlassIcon,
  FileCodeGlassIcon,
  FlaskConicalGlassIcon,
  GaugeGlassIcon,
  GitBranchGlassIcon,
  GitForkGlassIcon,
  GlobeGlassIcon,
  IdCardGlassIcon,
  IdCardLanyardGlassIcon,
  KeyGlassIcon,
  LayersGlassIcon,
  LayoutGridGlassIcon,
  LockGlassIcon,
  MailGlassIcon,
  PlugGlassIcon,
  RestoreGlassIcon,
  SettingsGlassIcon,
  Share2GlassIcon,
  ShareGlassIcon,
  ShieldCheckGlassIcon,
  ShieldGlassIcon,
  SlidersHorizontalGlassIcon,
  SmartphoneGlassIcon,
  SocialGithubGlassIcon,
  SocialGitlabGlassIcon,
  TerminalGlassIcon,
  UniversityGlassIcon,
  UploadGlassIcon,
  UserGlassIcon,
  UsersGlassIcon,
  WebhookGlassIcon,
  WrenchGlassIcon,
} from '@thesvg/ui-react'
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
        icon: UserGlassIcon,
      },
      {
        id: 'general',
        to: '/settings/general',
        icon: SettingsGlassIcon,
      },
      {
        id: 'organizations',
        to: '/settings/organizations',
        icon: UniversityGlassIcon,
      },
      {
        id: 'environments',
        to: '/settings/environments',
        icon: LayersGlassIcon,
        feature: 'global-environments',
      },
      {
        id: 'stacks',
        to: '/settings/stacks',
        icon: LayersGlassIcon,
        feature: 'iac',
      },
      {
        id: 'stack-templates',
        to: '/settings/stack-templates',
        icon: LayoutGridGlassIcon,
        feature: 'iac',
      },
      {
        id: 'import-platform',
        to: '/settings/import-platform',
        icon: DownloadGlassIcon,
      },
      {
        id: 'data-export',
        to: '/settings/data-export',
        icon: ShareGlassIcon,
      },
    ],
  },
  {
    id: 'access',
    items: [
      {
        id: 'users',
        to: '/settings/users',
        icon: UsersGlassIcon,
      },
      {
        id: 'teams',
        to: '/settings/teams',
        icon: UsersGlassIcon,
      },
      {
        id: 'roles',
        to: '/settings/roles',
        icon: IdCardLanyardGlassIcon,
        feature: 'access-roles',
      },
      {
        id: 'sso',
        to: '/settings/sso',
        icon: IdCardGlassIcon,
        feature: 'saml-scim',
      },
      {
        id: 'session-policy',
        to: '/settings/session-policy',
        icon: ClockGlassIcon,
      },
      {
        id: 'oauth',
        to: '/settings/oauth',
        icon: LockGlassIcon,
      },
      {
        id: 'tokens',
        to: '/settings/tokens',
        icon: KeyGlassIcon,
      },
      {
        id: 'mobile-devices',
        to: '/settings/mobile-devices',
        icon: SmartphoneGlassIcon,
        feature: 'mobile-api',
      },
      {
        id: 'plugins',
        to: '/settings/plugins',
        icon: PlugGlassIcon,
        feature: 'plugins',
      },
      {
        id: 'audit-log',
        to: '/settings/audit-log',
        icon: RestoreGlassIcon,
      },
    ],
  },
  {
    id: 'security',
    items: [
      {
        id: 'security',
        to: '/settings/security',
        icon: ShieldGlassIcon,
      },
      {
        id: 'security-center',
        to: '/security',
        icon: ShieldCheckGlassIcon,
      },
      {
        id: 'security-runtime',
        to: '/settings/security-runtime',
        icon: SlidersHorizontalGlassIcon,
      },
      {
        id: 'vault',
        to: '/settings/vault',
        icon: LockGlassIcon,
      },
      {
        id: 'firewall',
        to: '/settings/firewall',
        icon: ShieldGlassIcon,
      },
    ],
  },
  {
    id: 'developer',
    items: [
      {
        id: 'cli-access',
        to: '/settings/cli-access',
        icon: TerminalGlassIcon,
      },
      {
        id: 'api-explorer',
        to: '/settings/api-explorer',
        icon: CodeGlassIcon,
      },
    ],
  },
  {
    id: 'ai',
    items: [
      {
        id: 'agents',
        to: '/settings/agents',
        icon: PlugGlassIcon,
      },
      {
        id: 'ai-control',
        to: '/settings/ai-control',
        icon: ShieldCheckGlassIcon,
      },
      {
        id: 'ai-assistant',
        to: '/settings/ai-assistant',
        icon: BotGlassIcon,
        feature: 'ai-chat',
      },
    ],
  },
  {
    id: 'platform',
    items: [
      {
        id: 'platform',
        to: '/settings/platform',
        icon: SettingsGlassIcon,
      },
      {
        id: 'platform-https',
        to: '/settings/platform/https',
        icon: LockGlassIcon,
      },
      {
        id: 'platform-ingress',
        to: '/settings/platform/ingress',
        icon: PlugGlassIcon,
      },
      {
        id: 'platform-addresses',
        to: '/settings/platform/addresses',
        icon: GlobeGlassIcon,
      },
      {
        id: 'platform-dns-providers',
        to: '/settings/platform/dns-providers',
        icon: CloudCheckGlassIcon,
      },
    ],
  },
  {
    id: 'domains',
    items: [
      {
        id: 'domains',
        to: '/domains',
        icon: GlobeGlassIcon,
      },
      {
        id: 'certificates',
        to: '/settings/certificates',
        icon: ShieldCheckGlassIcon,
      },
      {
        id: 'cloudflare-tunnel',
        to: '/settings/cloudflare-tunnel',
        icon: CloudCheckGlassIcon,
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
        icon: UploadGlassIcon,
      },
      {
        id: 'storage',
        to: '/settings/storage',
        icon: UploadGlassIcon,
      },
      {
        id: 'control-plane-backup',
        to: '/settings/control-plane-backup',
        icon: DatabaseGlassIcon,
      },
      {
        id: 'dr-report',
        to: '/settings/dr-report',
        icon: ClipboardListGlassIcon,
      },
      {
        id: 'database-upgrades',
        to: '/settings/database-upgrades',
        icon: DatabaseGlassIcon,
      },
    ],
  },
  {
    id: 'notifications',
    items: [
      {
        id: 'notification-channels',
        to: '/settings/notification-channels',
        icon: WebhookGlassIcon,
      },
      {
        id: 'event-webhooks',
        to: '/settings/event-webhooks',
        icon: WebhookGlassIcon,
      },
      {
        id: 'traffic-export',
        to: '/settings/traffic-export',
        icon: Share2GlassIcon,
      },
      {
        id: 'email',
        to: '/settings/email',
        icon: MailGlassIcon,
      },
      {
        id: 'email-templates',
        to: '/settings/email-templates',
        icon: MailGlassIcon,
      },
      {
        id: 'email-audiences',
        to: '/settings/email-audiences',
        icon: UsersGlassIcon,
      },
      {
        id: 'email-messages',
        to: '/settings/email-messages',
        icon: MailGlassIcon,
      },
      {
        id: 'status-page',
        to: '/settings/status-page',
        icon: ActivityGlassIcon,
      },
      {
        id: 'maintenance',
        to: '/settings/maintenance',
        icon: WrenchGlassIcon,
      },
      {
        id: 'deploy-policies',
        to: '/settings/deploy-policies',
        icon: ShieldCheckGlassIcon,
      },
      {
        id: 'build-performance',
        to: '/settings/build-performance',
        icon: ClockGlassIcon,
      },
    ],
  },
  {
    id: 'integrations',
    items: [
      {
        id: 'git-connections',
        to: '/settings/git-connections',
        icon: GitBranchGlassIcon,
      },
      {
        id: 'github-app',
        to: '/settings/github-app',
        icon: SocialGithubGlassIcon,
      },
      {
        id: 'gitlab-app',
        to: '/settings/gitlab-app',
        icon: SocialGitlabGlassIcon,
      },
      {
        id: 'bitbucket-app',
        to: '/settings/bitbucket-app',
        icon: GitBranchGlassIcon,
      },
      {
        id: 'gitea-app',
        to: '/settings/gitea-app',
        icon: CoffeeGlassIcon,
      },
      {
        id: 'forgejo-app',
        to: '/settings/forgejo-app',
        icon: GitForkGlassIcon,
      },
      {
        id: 'bitbucket-server-app',
        to: '/settings/bitbucket-server-app',
        icon: GitBranchGlassIcon,
      },
    ],
  },
  {
    id: 'beta',
    items: [
      {
        id: 'beta-features',
        to: '/settings/beta-features',
        icon: FlaskConicalGlassIcon,
      },
    ],
  },
  {
    id: 'updates',
    items: [
      {
        id: 'updates',
        to: '/settings/updates',
        icon: ArrowUpGlassIcon,
      },
    ],
  },
  {
    id: 'advanced',
    items: [
      {
        id: 'containers',
        to: '/settings/containers',
        icon: LayersGlassIcon,
      },
      {
        id: 'cleanup',
        to: '/settings/cleanup',
        icon: BroomGlassIcon,
      },
      {
        id: 'infrastructure',
        to: '/settings/infrastructure',
        icon: FileCodeGlassIcon,
        feature: 'iac',
      },
      {
        id: 'observability',
        to: '/settings/observability',
        icon: ChartLineGlassIcon,
      },
      {
        id: 'system-status',
        to: '/settings/system-status',
        icon: ActivityGlassIcon,
      },
      {
        id: 'control-plane-health',
        to: '/settings/control-plane-health',
        icon: GaugeGlassIcon,
      },
      {
        id: 'benchmarks',
        to: '/settings/benchmarks',
        icon: ChartColumnGlassIcon,
      },
    ],
  },
]

// Gated pages stay listed; useSettingsNav marks the switched-off ones locked.
export function visibleSettingsSections(): SettingsNavSection[] {
  return settingsNavSections
}
