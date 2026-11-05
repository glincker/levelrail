import { z } from 'zod'
import { buildOpsgenieNotifyUrl } from './opsgenieNotifyUrl'
import { buildPushoverNotifyUrl } from './pushoverNotifyUrl'
import { buildResendNotifyUrl } from './resendNotifyUrl'
import type { NotificationChannelKind } from '../types/notificationChannel'

export const NOTIFICATION_CHANNEL_KIND_ORDER = [
  'slack',
  'discord',
  'telegram',
  'teams',
  'mattermost',
  'lark',
  'rocketchat',
  'webex',
  'googlechat',
  'generic',
  'email',
  'pushover',
  'pagerduty',
  'opsgenie',
  'resend',
  'ntfy',
  'gotify',
  'webpush',
] as const

// Destination placeholder and setup-guide link per kind: each URL is the
// platform's own official webhook/bot setup doc.
export const CHANNEL_KIND_META: Record<
  NotificationChannelKind,
  { placeholder: string; href?: string }
> = {
  slack: {
    placeholder: 'https://hooks.slack.com/services/...',
    href: 'https://api.slack.com/messaging/webhooks',
  },
  discord: {
    placeholder: 'https://discord.com/api/webhooks/...',
    href: 'https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks',
  },
  telegram: {
    placeholder: 'https://api.telegram.org/bot<token>/sendMessage?chat_id=...',
    href: 'https://core.telegram.org/bots/tutorial',
  },
  generic: { placeholder: 'https://example.com/webhook' },
  email: { placeholder: 'ops@example.com' },
  pushover: {
    placeholder: '',
    href: 'https://pushover.net/api#registration',
  },
  pagerduty: {
    placeholder: '',
    href: 'https://support.pagerduty.com/docs/services-and-integrations#events-api-v2',
  },
  teams: {
    placeholder: 'https://example.webhook.office.com/webhookb2/...',
    href: 'https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook',
  },
  mattermost: {
    placeholder: 'https://mattermost.example.com/hooks/...',
    href: 'https://developers.mattermost.com/integrate/webhooks/incoming/',
  },
  lark: {
    placeholder: 'https://open.larksuite.com/open-apis/bot/v2/hook/...',
    href: 'https://open.larksuite.com/document/client-docs/bot-v3/add-custom-bot',
  },
  resend: { placeholder: '' },
  ntfy: {
    placeholder: 'https://ntfy.sh/my-topic',
    href: 'https://docs.ntfy.sh/publish/',
  },
  gotify: {
    placeholder: 'https://gotify.example.com/message?token=...',
    href: 'https://gotify.net/docs/pushmsg',
  },
  rocketchat: {
    placeholder: 'https://rocketchat.example.com/hooks/...',
    href: 'https://docs.rocket.chat/docs/integrations#incoming-webhook-script',
  },
  opsgenie: { placeholder: '' },
  webex: {
    placeholder: 'https://webexapis.com/v1/webhooks/incoming/...',
    href: 'https://developer.webex.com/messaging/docs/api/guides/webhooks',
  },
  googlechat: {
    placeholder: 'https://chat.googleapis.com/v1/spaces/.../messages?key=...',
    href: 'https://developers.google.com/workspace/chat/quickstart/webhooks',
  },
  // No destination URL at all: BrowserPushField renders in its place
  // below, and resolveNotifyUrl always sends "" for this kind.
  webpush: { placeholder: '' },
}

export const createChannelSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    kind: z.enum(NOTIFICATION_CHANNEL_KIND_ORDER),
    notifyUrl: z.string().trim(),
    pushoverUserKey: z.string().trim(),
    pushoverApiToken: z.string().trim(),
    pagerdutyRoutingKey: z.string().trim(),
    resendApiKey: z.string().trim(),
    resendTo: z.string().trim(),
    resendFrom: z.string().trim(),
    opsgenieApiKey: z.string().trim(),
  })
  .superRefine((data, ctx) => {
    if (data.kind === 'pushover') {
      if (!data.pushoverUserKey) {
        ctx.addIssue({
          code: 'custom',
          message: 'User Key is required',
          path: ['pushoverUserKey'],
        })
      }
      if (!data.pushoverApiToken) {
        ctx.addIssue({
          code: 'custom',
          message: 'API Token is required',
          path: ['pushoverApiToken'],
        })
      }
      return
    }
    if (data.kind === 'pagerduty') {
      if (!data.pagerdutyRoutingKey) {
        ctx.addIssue({
          code: 'custom',
          message: 'Integration/Routing Key is required',
          path: ['pagerdutyRoutingKey'],
        })
      }
      return
    }
    if (data.kind === 'resend') {
      if (!data.resendApiKey) {
        ctx.addIssue({
          code: 'custom',
          message: 'API Key is required',
          path: ['resendApiKey'],
        })
      }
      if (!data.resendTo) {
        ctx.addIssue({
          code: 'custom',
          message: 'Destination address is required',
          path: ['resendTo'],
        })
      } else if (!z.email().safeParse(data.resendTo).success) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be a valid email address',
          path: ['resendTo'],
        })
      }
      if (data.resendFrom && !z.email().safeParse(data.resendFrom).success) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be a valid email address',
          path: ['resendFrom'],
        })
      }
      return
    }
    if (data.kind === 'opsgenie') {
      if (!data.opsgenieApiKey) {
        ctx.addIssue({
          code: 'custom',
          message: 'API Key is required',
          path: ['opsgenieApiKey'],
        })
      }
      return
    }
    // webpush has no destination field at all: readiness (has this
    // browser actually subscribed yet) is tracked as component state,
    // not a form field, and enforced in onSubmit below instead.
    if (data.kind === 'webpush') {
      return
    }
    if (!data.notifyUrl) {
      ctx.addIssue({
        code: 'custom',
        message: 'Destination is required',
        path: ['notifyUrl'],
      })
      return
    }
    const isValid =
      data.kind === 'email'
        ? z.email().safeParse(data.notifyUrl).success
        : z.url().safeParse(data.notifyUrl).success
    if (!isValid) {
      ctx.addIssue({
        code: 'custom',
        message:
          data.kind === 'email'
            ? 'Must be a valid email address'
            : 'Must be a valid URL',
        path: ['notifyUrl'],
      })
    }
  })

export type CreateChannelForm = z.infer<typeof createChannelSchema>

export const CHANNEL_DEFAULT_VALUES: CreateChannelForm = {
  name: '',
  kind: 'slack',
  notifyUrl: '',
  pushoverUserKey: '',
  pushoverApiToken: '',
  pagerdutyRoutingKey: '',
  resendApiKey: '',
  resendTo: '',
  resendFrom: '',
  opsgenieApiKey: '',
}

// The single notify_url string the API expects, per kind: Pushover's and
// Resend's credentials get packed into it client-side
// (buildPushoverNotifyUrl/buildResendNotifyUrl), PagerDuty's routing key
// is sent through as-is (it's the whole notify_url for that kind, not a
// URL), every other kind sends the destination field as-is.
export function resolveNotifyUrl(values: CreateChannelForm): string {
  if (values.kind === 'pushover') {
    return buildPushoverNotifyUrl(
      values.pushoverUserKey.trim(),
      values.pushoverApiToken.trim(),
    )
  }
  if (values.kind === 'pagerduty') {
    return values.pagerdutyRoutingKey.trim()
  }
  if (values.kind === 'resend') {
    return buildResendNotifyUrl(
      values.resendApiKey.trim(),
      values.resendTo.trim(),
      values.resendFrom.trim() || undefined,
    )
  }
  if (values.kind === 'opsgenie') {
    return buildOpsgenieNotifyUrl(values.opsgenieApiKey.trim())
  }
  // webpush: every registered browser subscription is the destination,
  // so notify_url is unused server-side; the empty string is valid.
  if (values.kind === 'webpush') {
    return ''
  }
  return values.notifyUrl.trim()
}
