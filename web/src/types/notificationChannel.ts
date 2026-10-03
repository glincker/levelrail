// Wire types for the notification-channel resource, matching
// internal/api/notification_channels.go's notificationChannelResource.

export type NotificationChannelKind =
  | 'generic'
  | 'slack'
  | 'discord'
  | 'telegram'
  | 'email'
  | 'pushover'
  | 'pagerduty'
  | 'teams'
  | 'resend'
  | 'ntfy'
  | 'gotify'
  | 'mattermost'
  | 'lark'
  | 'rocketchat'
  | 'opsgenie'
  | 'webex'
  | 'googlechat'
  | 'webpush'

export interface NotificationChannel {
  id: string
  name: string
  kind: NotificationChannelKind
  notify_url: string
  enabled: boolean
  // Slack/Discord-only opt-in: real Approve/Deny buttons on a deploy
  // approval message. has_interactive_secret is true once a secret is
  // stored; the secret itself is write-only and never echoed back.
  interactive_approvals: boolean
  has_interactive_secret: boolean
  created_at: string
  updated_at: string
}

export interface CreateNotificationChannelRequest {
  name: string
  kind: NotificationChannelKind
  notify_url: string
  enabled?: boolean
  // Must be resent on every update while interactive_approvals stays
  // true: this request body is a full replace, matching notify_url's
  // own convention above, not a partial patch.
  interactive_approvals?: boolean
  interactive_secret?: string
}

export interface TestNotificationChannelRequest {
  kind: NotificationChannelKind
  notify_url: string
}

// Matches internal/api's notificationDeliveryResource
// (internal/api/notification_channels.go).
export interface NotificationDelivery {
  id: string
  channel_id: string
  trigger: string
  success: boolean
  error?: string
  created_at: string
}
