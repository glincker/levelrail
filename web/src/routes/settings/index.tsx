import { createFileRoute, Link } from '@tanstack/react-router'
import type { ComponentType } from 'react'
import {
  UserIcon,
  ShieldIcon,
  KeyIcon,
  LockKeyIcon,
  UsersIcon,
  GithubLogoIcon,
  GitlabLogoIcon,
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
import { Card, CardHeader, CardTitle, CardDescription } from '../../components/ui/card'

export const Route = createFileRoute('/settings/')({
  component: SettingsHubPage,
})

interface SettingsCardDef {
  to: string
  icon: ComponentType<{ className?: string }>
  title: string
  description: string
}

interface SettingsSection {
  heading: string
  cards: SettingsCardDef[]
}

const sections: SettingsSection[] = [
  {
    heading: 'Account',
    cards: [
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
    cards: [
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
    ],
  },
  {
    heading: 'Integrations',
    cards: [
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
    cards: [
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

function SettingsHubPage() {
  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Settings</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Account, team, and platform configuration.
        </p>
      </div>

      {sections.map((section) => (
        <div key={section.heading} className="space-y-3">
          <h2 className="text-sm font-medium text-muted-foreground">
            {section.heading}
          </h2>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {section.cards.map((card) => (
              <Link key={card.to} to={card.to} className="block">
                <Card className="h-full transition-colors hover:ring-foreground/20">
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <card.icon className="size-4" />
                      {card.title}
                    </CardTitle>
                    <CardDescription>{card.description}</CardDescription>
                  </CardHeader>
                </Card>
              </Link>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
