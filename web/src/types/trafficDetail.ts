// Wire types for GET /api/v1/apps/{name}/traffic
// (internal/api/traffic_app.go's trafficResponse).

export type TrafficWindow = '5m' | '1h' | '24h' | '7d'

export interface TrafficLabel {
  label: string
  requests: number
  share: number
}

export interface TrafficLatency {
  p50_ms: number
  p95_ms: number
  p99_ms: number
}

export interface TrafficReplica extends TrafficLatency {
  upstream: string
  requests: number
  rate_per_sec: number
  error_rate_4xx: number
  error_rate_5xx: number
  upstream_errors: number
}

export interface TrafficRoute {
  route: string
  requests: number
  error_rate_4xx: number
  error_rate_5xx: number
  avg_ms: number
}

export interface TrafficSample {
  time: string
  route: string
  method: string
  status: number
  duration_ms: number
  upstream?: string
  request_id?: string
}

export interface TrafficSLO {
  objective: string
  target: number
  latency_ms?: number
  burn_rate: number
  budget_remaining: number
}

export interface TrafficShieldReason {
  reason: 'rate_limit' | 'concurrency' | 'bad_bot' | 'body_too_large'
  count: number
}

export interface TrafficShield {
  blocked: number
  by_reason: TrafficShieldReason[]
}

export interface TrafficErrorSpike {
  start: string
  end: string
  requests: number
  errors_5xx: number
  peak_error_rate_5xx: number
  baseline_error_rate_5xx: number
  deploy_id?: string
  deploy_at?: string
  deploy_status?: string
  deploy_image?: string
  deploy_commit?: string
}

export interface TrafficDetail {
  app: string
  window: TrafficWindow
  from: string
  to: string
  has_traffic: boolean
  requests: number
  rate_per_sec: number
  status_classes: { '2xx': number; '3xx': number; '4xx': number; '5xx': number }
  error_rate_4xx: number
  error_rate_5xx: number
  upstream_errors: number
  latency: TrafficLatency
  replicas: TrafficReplica[]
  slow_routes: TrafficRoute[]
  top_routes: TrafficRoute[]
  samples: TrafficSample[]
  referrers: TrafficLabel[]
  countries: TrafficLabel[]
  slo?: TrafficSLO
  shield: TrafficShield
  error_spikes: TrafficErrorSpike[]
}
