import type { FileRouteTypes } from '@/routeTree.gen'
import type relatedLinksEn from '../locales/en/relatedLinks.json'

export type RelatedTo = FileRouteTypes['to']

export interface RelatedLinkEntry {
  to: RelatedTo
  /** Key under `links` in the relatedLinks i18n namespace. */
  labelKey: keyof (typeof relatedLinksEn)['links']
  /** Reuse the current route params (app name) for the target. */
  inheritParams?: boolean
}

// Keys are route ids without a trailing slash; targets are static paths so
// a removed route fails typecheck here.
export const RELATED_LINKS: Readonly<Record<string, RelatedLinkEntry[]>> = {
  '/settings/account': [
    { to: '/settings/security', labelKey: 'security' },
    { to: '/settings/session-policy', labelKey: 'sessionPolicy' },
    { to: '/settings/mobile-devices', labelKey: 'mobileDevices' },
    { to: '/settings/cli-access', labelKey: 'cliAccess' },
  ],
  '/settings/tokens': [
    { to: '/settings/account', labelKey: 'account' },
    { to: '/settings/cli-access', labelKey: 'cliAccess' },
  ],
  '/settings/cli-access': [{ to: '/settings/tokens', labelKey: 'tokens' }],
  '/settings/security': [
    { to: '/settings/account', labelKey: 'account' },
    { to: '/settings/session-policy', labelKey: 'sessionPolicy' },
  ],
  '/settings/notification-channels': [
    { to: '/alerts', labelKey: 'alertRules' },
  ],
  '/alerts': [
    { to: '/settings/notification-channels', labelKey: 'notificationChannels' },
  ],
  '/domains/$domain': [
    { to: '/settings/platform/https', labelKey: 'platformHttps' },
    { to: '/network', labelKey: 'network' },
  ],
  '/settings/certificates': [
    { to: '/domains', labelKey: 'domains' },
    { to: '/settings/platform/https', labelKey: 'platformHttps' },
  ],
  '/apps/$name/environment': [
    { to: '/settings/vault', labelKey: 'vault' },
    { to: '/settings/environments', labelKey: 'environments' },
  ],
  '/apps/$name/domains': [
    { to: '/domains', labelKey: 'domains' },
    { to: '/apps/$name/traffic', labelKey: 'traffic', inheritParams: true },
  ],
  '/apps/$name/network': [
    { to: '/network/private', labelKey: 'privateNetworks' },
    { to: '/firewall', labelKey: 'firewall' },
  ],
  '/settings/backup-targets': [
    { to: '/backups', labelKey: 'backups' },
    { to: '/settings/dr-report', labelKey: 'drReport' },
  ],
  '/settings/audit-log': [
    { to: '/settings/data-export', labelKey: 'dataExport' },
    { to: '/settings/event-webhooks', labelKey: 'eventWebhooks' },
  ],
}

export function relatedLinksFor(routeId: string): RelatedLinkEntry[] {
  const key = routeId.length > 1 ? routeId.replace(/\/$/, '') : routeId
  return RELATED_LINKS[key] ?? []
}
