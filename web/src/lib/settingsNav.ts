import type { ComponentType } from 'react'
import {
  UserIcon,
  ShieldIcon,
  KeyIcon,
  LockKeyIcon,
  UsersIcon,
  BuildingsIcon,
  GithubLogoIcon,
  GitlabLogoIcon,
  GitBranchIcon,
  WebhooksLogoIcon,
  CloudArrowUpIcon,
  CloudCheckIcon,
  EnvelopeIcon,
  GearIcon,
  GlobeIcon,
  ArrowCircleUpIcon,
  ClockCounterClockwiseIcon,
  PackageIcon,
} from '@phosphor-icons/react/dist/ssr'

export interface SettingsNavItem {
  to: string
  icon: ComponentType<{ className?: string }>
  title: string
  description: string
}

export interface SettingsNavSection {
  heading: string
  items: SettingsNavItem[]
}

// Single source of truth for every settings page: the hub's card grid
// (routes/settings/index.tsx) and the sidebar's sub-nav
// (SettingsScopedSidebar.tsx) both render from this list, so a new
// settings page only needs adding here once.
export const settingsNavSections: SettingsNavSection[] = [
  {
    heading: 'Account',
    items: [
      {
        to: '/settings/account',
        icon: UserIcon,
        title: 'Account',
        description: 'Profile and password.',
      },
      {
        to: '/settings/security',
        icon: ShieldIcon,
        title: 'Security',
        description: 'Sessions and login protection.',
      },
      {
        to: '/settings/tokens',
        icon: KeyIcon,
        title: 'API tokens',
        description: 'Scoped, revocable credentials for the CLI, CI, and MCP.',
      },
    ],
  },
  {
    heading: 'Team',
    items: [
      {
        to: '/settings/users',
        icon: UsersIcon,
        title: 'Users',
        description: 'Everyone with access to this platform.',
      },
      {
        to: '/settings/oauth',
        icon: LockKeyIcon,
        title: 'OAuth sign-in',
        description: 'Let people sign in with Google or GitHub.',
      },
      {
        to: '/settings/organizations',
        icon: BuildingsIcon,
        title: 'Organizations',
        description: 'Group related projects under an organization.',
      },
    ],
  },
  {
    heading: 'Integrations',
    items: [
      {
        to: '/settings/github-app',
        icon: GithubLogoIcon,
        title: 'GitHub App',
        description: 'Private-repository access for git-based deploys.',
      },
      {
        to: '/settings/gitlab-app',
        icon: GitlabLogoIcon,
        title: 'GitLab App',
        description: 'gitlab.com or self-hosted project access for git-based deploys.',
      },
      {
        to: '/settings/bitbucket-app',
        icon: GitBranchIcon,
        title: 'Bitbucket App',
        description: 'Bitbucket Cloud repository access for git-based deploys.',
      },
      {
        to: '/settings/notification-channels',
        icon: WebhooksLogoIcon,
        title: 'Notification channels',
        description: 'Slack, Discord, Telegram, webhook, and email alerts.',
      },
      {
        to: '/settings/backup-targets',
        icon: CloudArrowUpIcon,
        title: 'Backup targets',
        description: 'S3-compatible buckets for managed database backups.',
      },
      {
        to: '/settings/registry-credentials',
        icon: PackageIcon,
        title: 'Registry credentials',
        description: 'Pull private images with build.type: image.',
      },
      {
        to: '/settings/email',
        icon: EnvelopeIcon,
        title: 'Email',
        description: 'Outbound SMTP for alerts and password resets.',
      },
      {
        to: '/settings/cloudflare-tunnel',
        icon: CloudCheckIcon,
        title: 'Cloudflare Tunnel',
        description: 'Expose this control plane without opening an inbound port.',
      },
    ],
  },
  {
    heading: 'Platform',
    items: [
      {
        to: '/settings/general',
        icon: GearIcon,
        title: 'General',
        description: 'System status and configuration.',
      },
      {
        to: '/domains',
        icon: GlobeIcon,
        title: 'Domains',
        description: 'Platform ingress: dashboard domain and ACME certificates.',
      },
      {
        to: '/settings/updates',
        icon: ArrowCircleUpIcon,
        title: 'Updates',
        description: 'Current version and available releases.',
      },
      {
        to: '/settings/audit-log',
        icon: ClockCounterClockwiseIcon,
        title: 'Audit log',
        description: 'Who changed what, across every session and API token.',
      },
    ],
  },
]
