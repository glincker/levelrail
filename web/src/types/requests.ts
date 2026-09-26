// Wire types for GET /api/v1/apps/{name}/requests
// (internal/api/requests.go's requestsResponse).

export interface RequestPoint {
  timestamp: string
  requests: number
  rate_per_sec: number
  error_rate_4xx: number
  error_rate_5xx: number
  upstream_errors: number
  p50_ms: number
  p95_ms: number
  p99_ms: number
  bytes_in_per_sec: number
  bytes_out_per_sec: number
}

export interface RequestSummary {
  window_seconds: number
  has_traffic: boolean
  requests: number
  rate_per_sec: number
  error_rate_4xx: number
  error_rate_5xx: number
  p95_ms: number
  upstream_errors: number
}

export interface RequestSeries {
  app: string
  from: string
  to: string
  step_seconds: number
  summary: RequestSummary
  points: RequestPoint[]
}
