// Wire type for GET /api/v1/apps/resource-usage
// (internal/api/app_resource_usage.go's handleAppResourceUsage): the
// latest known CPU/memory/network reading for every app in one
// response, the data behind the dashboard's "top resource consumers"
// ranking. A field is undefined when telemetry has never recorded that
// metric for this app yet (a fresh deploy, or no telemetry configured
// at all), not a real zero reading.
export interface AppResourceUsage {
  name: string
  cpu_percent?: number
  memory_usage_bytes?: number
  memory_limit_bytes?: number
  network_rx_bytes?: number
  network_tx_bytes?: number
}
