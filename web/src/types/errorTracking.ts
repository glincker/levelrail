// Wire types for internal/api/errors_issues.go.

export type ErrorStatus = 'open' | 'resolved' | 'ignored'

export interface ErrorIssue {
  fingerprint: string
  title: string
  level: string
  first_seen_ns: number
  last_seen_ns: number
  count: number
  last_release?: string
  last_deploy_id?: string
  status: ErrorStatus
  regressed: boolean
}

export interface ErrorFrame {
  filename?: string
  module?: string
  function?: string
  lineno?: number
  in_app?: boolean
}

export interface ErrorException {
  type?: string
  value?: string
  stacktrace?: { frames?: ErrorFrame[] }
}

// The scrubbed Sentry event as stored; only the parts the UI reads.
export interface ErrorEventDoc {
  message?: string | { formatted?: string; message?: string }
  exception?: { values?: ErrorException[] } | ErrorException[]
  request?: { url?: string; method?: string }
  tags?: Record<string, string>
}

export interface ErrorEvent {
  event_id: string
  timestamp_ns: number
  release?: string
  environment?: string
  deploy_id?: string
  event?: ErrorEventDoc
}

export interface ErrorIssueDetail {
  issue: ErrorIssue
  events: ErrorEvent[]
}

export interface ErrorTracking {
  enabled: boolean
  endpoint: string
  // Present only in the response that created or rotated the key.
  dsn?: string
  env?: Record<string, string>
}
