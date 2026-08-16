// Wire types for the notification-channel resource, matching
// internal/api/notification_channels.go's notificationChannelResource.

export type NotificationChannelKind =
  'generic' | 'slack' | 'discord' | 'telegram' | 'email'

export interface NotificationChannel {
  id: string
  name: string
  kind: NotificationChannelKind
  notify_url: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateNotificationChannelRequest {
  name: string
  kind: NotificationChannelKind
  notify_url: string
  enabled?: boolean
}

export interface TestNotificationChannelRequest {
  kind: NotificationChannelKind
  notify_url: string
}
