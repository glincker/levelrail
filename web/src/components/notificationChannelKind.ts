// Shared kind display metadata for notification channels, mirroring
// backupTargetProvider.ts's own PROVIDER_LABEL shape exactly, used by
// both the connect dialog's picker and the settings table.

import type { NotificationChannelKind } from '../types/notificationChannel'
import type { BrandIconName } from './BrandIcon'
import { INTEGRATION_GLYPHS } from '../lib/integrationBrands'

export const CHANNEL_KIND_LABEL: Record<NotificationChannelKind, string> = {
  slack: 'Slack',
  discord: 'Discord',
  telegram: 'Telegram',
  generic: 'Generic webhook',
  email: 'Email',
  pushover: 'Pushover',
  pagerduty: 'PagerDuty',
  teams: 'Microsoft Teams',
  resend: 'Resend',
  ntfy: 'ntfy',
  gotify: 'Gotify',
  mattermost: 'Mattermost',
  lark: 'Lark',
  rocketchat: 'Rocket.Chat',
  opsgenie: 'Opsgenie',
  webex: 'Webex',
  googlechat: 'Google Chat',
  webpush: 'Browser push',
}

// Kinds with a brand mark, derived from the shared integration mapping; the
// rest (generic, email, webpush) render through a UI icon chosen by the caller.
export const CHANNEL_KIND_BRAND_ICON: Partial<
  Record<NotificationChannelKind, BrandIconName>
> = Object.fromEntries(
  Object.entries(INTEGRATION_GLYPHS.notify).flatMap(([kind, spec]) =>
    spec.brand ? [[kind, spec.brand]] : [],
  ),
)
