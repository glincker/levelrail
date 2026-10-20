// Wire types for internal/api/event_webhooks.go. The signing secret appears
// only on create and rotate responses and is never returned again.

export type EventDeliveryState = 'pending' | 'delivered' | 'dead'

export interface EventWebhook {
  id: string
  name: string
  url: string
  enabled: boolean
  events: string[]
  created_at: string
  updated_at: string
  pending: number
  delivered: number
  dead: number
  last_status: number
  signing_secret?: string
}

export interface EventWebhookDelivery {
  id: string
  webhook_id: string
  event_id: string
  event_type: string
  state: EventDeliveryState
  attempts: number
  response_status: number
  last_error?: string
  next_attempt_at: string
  created_at: string
  updated_at: string
}

export interface EventWebhookTestResult {
  ok: boolean
  status?: number
  error?: string
}

export interface CreateEventWebhookRequest {
  name: string
  url: string
  enabled?: boolean
  events?: string[]
}

export interface UpdateEventWebhookRequest {
  name?: string
  url?: string
  enabled?: boolean
  events?: string[]
}
