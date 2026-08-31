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
