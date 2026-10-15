// Wire types for internal/api/traces.go.

export interface TraceSummary {
  trace_id: string
  root_name: string
  start_ns: number
  duration_ns: number
  spans: number
  errors: number
}

export interface TraceSpan {
  trace_id: string
  span_id: string
  parent_id?: string
  name: string
  kind: number
  start_ns: number
  duration_ns: number
  status_code: number
  status_message?: string
  attributes?: Record<string, string | number | boolean>
}

export interface Tracing {
  enabled: boolean
  endpoint: string
  header: string
  // Present only in the response that created or rotated the key.
  key?: string
  env?: Record<string, string>
}
