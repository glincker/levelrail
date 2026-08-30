// Shared kind display metadata for notification channels, mirroring
// backupTargetProvider.ts's own PROVIDER_LABEL shape exactly, used by
// both the connect dialog's picker and the settings table.

import type { NotificationChannelKind } from '../types/notificationChannel'
import type { BrandIconName } from './BrandIcon'

export const CHANNEL_KIND_LABEL: Record<NotificationChannelKind, string> = {
  slack: 'Slack',
  discord: 'Discord',
  telegram: 'Telegram',
  generic: 'Generic webhook',
  email: 'Email',
  pushover: 'Pushover',
  pagerduty: 'PagerDuty',
  teams: 'Microsoft Teams',
}

// Only the brand-mark kinds map to a BrandIconName; generic/email/pushover
// render through a Phosphor icon instead (BrandIcon.tsx's own documented
// brand-vs-chrome boundary), chosen by the caller.
export const CHANNEL_KIND_BRAND_ICON: Partial<
  Record<NotificationChannelKind, BrandIconName>
> = {
  slack: 'slack',
  discord: 'discord',
  telegram: 'telegram',
  pagerduty: 'pagerduty',
  teams: 'microsoft-teams',
}
