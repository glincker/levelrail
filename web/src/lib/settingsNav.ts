import type { ComponentType } from 'react'
import {
  UserIcon,
  ShieldIcon,
  ShieldCheckIcon,
  KeyIcon,
  LockKeyIcon,
  UsersIcon,
  BuildingsIcon,
  GithubLogoIcon,
  GitlabLogoIcon,
  GitBranchIcon,
  TeaBagIcon,
  WebhooksLogoIcon,
  CloudArrowUpIcon,
  CloudCheckIcon,
  DatabaseIcon,
  EnvelopeIcon,
  GearIcon,
  GlobeIcon,
  ArrowCircleUpIcon,
  ClockCounterClockwiseIcon,
  PackageIcon,
  DownloadSimpleIcon,
  TerminalWindowIcon,
  CodeIcon,
  HeartbeatIcon,
  StackIcon,
  HardDrivesIcon,
  RobotIcon,
  PlugsConnectedIcon,
  SparkleIcon,
  FileCodeIcon,
  ChartLineIcon,
  IdentificationBadgeIcon,
} from '@phosphor-icons/react/dist/ssr'
import { filterByFeature, type ExperimentalFeature } from './experimental'

export interface SettingsNavItem {
  to: string
  icon: ComponentType<{ className?: string }>
  title: string
  description: string
  feature?: ExperimentalFeature
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
      // Access-control surface bigger than its old spot implied: kept near
      // the top of Account rather than buried after CLI access.
      {
        to: '/settings/iam-policies',
        icon: ShieldCheckIcon,
        title: 'IAM policies',
        description:
          'Resource-scoped Allow/Deny access, layered on top of a token’s own abilities.',
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
      {
        to: '/settings/agents',
        icon: PlugsConnectedIcon,
        title: 'Agents',
        description: 'Connect an AI agent over MCP and manage its tokens.',
      },
      {
        to: '/settings/cli-access',
        icon: TerminalWindowIcon,
        title: 'CLI access',
        description:
          'Approve or deny logins started with levelrail-cli auth login.',
      },
      {
        to: '/settings/api-explorer',
        icon: CodeIcon,
        title: 'API explorer',
        description:
          'Browse and try real endpoints without leaving the dashboard.',
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
        to: '/settings/roles',
        icon: IdentificationBadgeIcon,
        title: 'Roles',
        description: 'Named ability sets, including the guest role.',
        feature: 'access-roles',
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
      {
        to: '/settings/environments',
        icon: StackIcon,
        title: 'Environments',
        description:
          'Dev, test, UAT and production environments, and which are protected.',
        feature: 'global-environments',
      },
    ],
  },
  {
    heading: 'Git providers',
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
        description:
          'gitlab.com or self-hosted project access for git-based deploys.',
      },
      {
        to: '/settings/bitbucket-app',
        icon: GitBranchIcon,
        title: 'Bitbucket App',
        description: 'Bitbucket Cloud repository access for git-based deploys.',
      },
      {
        to: '/settings/gitea-app',
        icon: TeaBagIcon,
        title: 'Gitea App',
        description:
          'Self-hosted Gitea repository access for git-based deploys.',
      },
    ],
  },
  {
    heading: 'Notifications & status',
    items: [
      {
        to: '/settings/notification-channels',
        icon: WebhooksLogoIcon,
        title: 'Notification channels',
        description: 'Slack, Discord, Telegram, webhook, and email alerts.',
      },
      {
        to: '/settings/status-page',
        icon: HeartbeatIcon,
        title: 'Status page',
        description:
          'Optional public page with component status, uptime and incidents.',
      },
    ],
  },
  {
    heading: 'Storage & backups',
    items: [
      {
        to: '/settings/backup-targets',
        icon: CloudArrowUpIcon,
        title: 'Backup targets',
        description: 'S3-compatible buckets for managed database backups.',
      },
      {
        to: '/settings/storage',
        icon: CloudArrowUpIcon,
        title: 'Storage destinations',
        description:
          'AWS S3, R2, B2, MinIO, Wasabi buckets for log archives and backups.',
      },
    ],
  },
  {
    // Named to avoid colliding with "Infrastructure as code" under Platform.
    heading: 'Registries & nodes',
    items: [
      {
        to: '/settings/registry-credentials',
        icon: PackageIcon,
        title: 'Registry credentials',
        description: 'Pull private images with build.type: image.',
      },
      {
        to: '/settings/node-providers',
        icon: HardDrivesIcon,
        title: 'Cloud node providers',
        description:
          'Hetzner and DigitalOcean tokens for automatic node provisioning.',
      },
      {
        to: '/settings/registry',
        icon: HardDrivesIcon,
        title: 'Container registry',
        description: 'Built-in image registry for multi-node build caching.',
      },
    ],
  },
  {
    heading: 'Platform extras',
    items: [
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
        description:
          'Expose this control plane without opening an inbound port.',
        feature: 'cloudflare-tunnel',
      },
      {
        to: '/settings/vault',
        icon: LockKeyIcon,
        title: 'Vault',
        description:
          'Resolve app secrets live from an external HashiCorp Vault instance.',
      },
      {
        to: '/settings/observability',
        icon: ChartLineIcon,
        title: 'Observability',
        description:
          'Remote-read connection info and a link out to your own Grafana.',
      },
      {
        to: '/settings/ai-assistant',
        icon: RobotIcon,
        title: 'AI Assistant',
        description:
          'Bring your own LLM API key for the platform chat assistant.',
        feature: 'ai-chat',
      },
      {
        to: '/settings/import-platform',
        icon: DownloadSimpleIcon,
        title: 'Import from another platform',
        description: 'Bring apps over from Coolify, Dokploy or CapRover.',
      },
    ],
  },
  {
    heading: 'Platform',
    items: [
      {
        to: '/settings/setup-wizard',
        icon: SparkleIcon,
        title: 'Setup wizard',
        description: 'Server checks, dashboard domain, git, and a first app.',
      },
      {
        to: '/settings/general',
        icon: GearIcon,
        title: 'General',
        description: 'System status and configuration.',
      },
      {
        to: '/settings/control-plane-backup',
        icon: DatabaseIcon,
        title: 'Control plane backup',
        description:
          'Snapshots, off-box backups, key escrow and restore drills for this instance.',
      },
      {
        to: '/settings/system-status',
        icon: HeartbeatIcon,
        title: 'System status',
        description:
          'Preflight checks: Docker, disk, ports, database, and firewall.',
      },
      {
        to: '/settings/firewall',
        icon: ShieldIcon,
        title: 'Firewall',
        description:
          'Allow or deny rules by port, protocol, and source, reconciled onto this node.',
      },
      {
        to: '/settings/containers',
        icon: StackIcon,
        title: 'Containers',
        description:
          'Every container on this node, managed by this platform or not.',
      },
      {
        to: '/domains',
        icon: GlobeIcon,
        title: 'Domains',
        description:
          'Platform ingress: dashboard domain and ACME certificates.',
      },
      {
        to: '/settings/certificates',
        icon: ShieldCheckIcon,
        title: 'TLS certificates',
        description:
          'Every domain’s certificate in one place: expiry, issuer, renew now, and custom certificate upload.',
      },
      {
        to: '/settings/infrastructure',
        icon: FileCodeIcon,
        title: 'Infrastructure as code',
        description:
          'Plan and apply YAML resource files, and export live state.',
        feature: 'iac',
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

export function visibleSettingsSections(
  enabled: readonly string[],
): SettingsNavSection[] {
  return settingsNavSections
    .map((s) => ({ ...s, items: filterByFeature(s.items, enabled) }))
    .filter((s) => s.items.length > 0)
}
