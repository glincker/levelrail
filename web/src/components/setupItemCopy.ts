import type { SetupItemId } from '../lib/setupChecklist'

export interface ItemCopy {
  title: string
  why: string
  cta: string
  to: string
}

export const ITEM_COPY: Record<SetupItemId, ItemCopy> = {
  source: {
    title: 'Connect a Git provider',
    why: 'Deploy on every push and get commit-linked deploy history.',
    cta: 'Connect',
    to: '/settings/github-app',
  },
  app: {
    title: 'Deploy your first app',
    why: 'Prove the build, deploy and rollback loop works end to end.',
    cta: 'Create app',
    to: '/apps',
  },
  domain: {
    title: 'Add a custom domain with a valid certificate',
    why: 'Serve your app over HTTPS on your own hostname.',
    cta: 'Domains',
    to: '/domains',
  },
  backup: {
    title: 'Add a backup target and create a control plane backup',
    why: 'Losing the control plane database means losing every app definition.',
    cta: 'Backups',
    to: '/settings/backup-targets',
  },
  alerts: {
    title: 'Create a notification channel and an alert rule',
    why: 'Hear about crashloops and expiring certificates before your users do.',
    cta: 'Channels',
    to: '/settings/notification-channels',
  },
  twoFactor: {
    title: 'Enable two-factor authentication',
    why: 'Protect the admin account that controls every deployment.',
    cta: 'Security',
    to: '/settings/security',
  },
  dashboardUrl: {
    title: 'Set the dashboard URL',
    why: 'Links in notifications and emails need a public address to point at.',
    cta: 'Settings',
    to: '/settings/general',
  },
}
