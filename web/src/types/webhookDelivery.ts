// Wire type for GET /api/v1/apps/{name}/webhook-deliveries
// (internal/api/webhook_deliveries.go's webhookDeliveryResource): one
// recorded inbound git provider webhook request, verified or not,
// matched to a connected git source or not.
export interface WebhookDelivery {
  id: string
  service_name: string
  provider: string
  event_type: string
  signature_valid: boolean
  matched: boolean
  status_code: number
  payload: string
  payload_truncated: boolean
  error?: string
  received_at: string
}

// Wire type for POST .../webhook-deliveries/{id}/replay
// (internal/api's replayWebhookDeliveryResult).
export interface ReplayWebhookDeliveryResult {
  status: number
  message: string
}
