export interface MonitorStatusRange {
  min: number
  max: number
}

export interface Monitor {
  id: string
  name: string
  kind: 'http' | 'tcp'
  interval_seconds: number
  timeout_ms: number
  enabled: boolean
  url?: string
  method?: string
  header_names?: string[]
  expect?: MonitorStatusRange[]
  contains?: string
  follow_redirects?: boolean
  host?: string
  port?: number
  created_at: string
  state?: 'up' | 'suspect' | 'down' | 'recovering'
  consecutive_failures?: number
  consecutive_successes?: number
}

export interface MonitorResult {
  at: number
  ok: boolean
  latency_ms: number
  code?: number
  reason?: string
}

export interface MonitorRollup {
  bucket: number
  total: number
  failed: number
  avg_latency_ms: number
  max_latency_ms: number
}

export interface MonitorResults {
  monitor_id: string
  raw: MonitorResult[]
  rollup: MonitorRollup[]
}

export interface CreateMonitorInput {
  name: string
  kind: 'http' | 'tcp'
  interval_seconds: number
  timeout_ms: number
  url?: string
  host?: string
  port?: number
  status_link?: boolean
}
