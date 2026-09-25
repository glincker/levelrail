import * as React from 'react'
import {
  GaugeIcon,
  StackIcon,
  DatabaseIcon,
  FolderIcon,
  BuildingsIcon,
  HardDrivesIcon,
  GlobeIcon,
  UserIcon,
  ShieldIcon,
  KeyIcon,
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
} from '@phosphor-icons/react/dist/ssr'

export interface PaletteItem {
  key: string
  label: string
  group: string
  icon: React.ReactNode
  run: () => void
}

export interface RouteEntry {
  key: string
  label: string
  group: string
  icon: React.ReactNode
  to: string
}

export const GROUP_ORDER = [
  'Recent',
  'Actions',
  'App actions',
  'Navigate',
  'Settings',
  'Apps',
  'Databases',
] as const

const nav = (
  key: string,
  label: string,
  icon: React.ReactNode,
  to: string,
  group = 'Navigate',
): RouteEntry => ({ key, label, group, icon, to })

export const ROUTE_ENTRIES: RouteEntry[] = [
  nav('action-status', 'Go to Status', <HeartbeatIcon />, '/status', 'Actions'),
  nav('action-apps', 'Go to Apps', <StackIcon />, '/apps', 'Actions'),
  nav('action-nodes', 'Go to Nodes', <HardDrivesIcon />, '/nodes', 'Actions'),
  nav('action-create-app', 'Create app', <PlusIcon />, '/apps', 'Actions'),
  nav(
    'action-templates',
    'Browse templates',
    <SquaresFourIcon />,
    '/apps',
    'Actions',
  ),
  nav('nav-dashboard', 'Dashboard', <GaugeIcon />, '/'),
  nav('nav-databases', 'Databases', <DatabaseIcon />, '/databases'),
  nav('nav-projects', 'Projects', <FolderIcon />, '/projects'),
  nav('nav-domains', 'Domains', <GlobeIcon />, '/domains'),
  nav(
    'nav-loadbalancers',
    'Load balancers',
    <ArrowsSplitIcon />,
    '/loadbalancers',
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
    'settings-registry-credentials',
    'Registry credentials',
    <PackageIcon />,
    '/settings/registry-credentials',
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

export const THEME_ACTION = {
  key: 'action-toggle-theme',
  label: 'Toggle theme',
  group: 'Actions',
  icon: <CircleHalfIcon />,
} as const
