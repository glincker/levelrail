// Wire types for the log explorer endpoints (internal/api/log_signatures.go,
// internal/api/log_saved_queries.go).

export interface LogSignature {
  id: string
  template: string
  level: string
  count: number
  first_seen: string
  last_seen: string
  buckets: number[]
  sample: string
  new?: boolean
}

export interface LogSignatures {
  from: string
  to: string
  scanned: number
  max_rows: number
  truncated: boolean
  total: number
  new_count: number
  has_baseline: boolean
  signatures: LogSignature[]
}

export interface LogQuerySpec {
  q?: string
  level?: string
  stream?: 'stdout' | 'stderr'
  fields?: string[]
  container?: string
  range?: string
  from?: string
  to?: string
}

export type SavedLogQueryVisibility = 'private' | 'instance'

export interface SavedLogQuery {
  id: string
  name: string
  app?: string
  spec: LogQuerySpec
  visibility: SavedLogQueryVisibility
  pinned: boolean
  owner?: string
  mine: boolean
  created_at: string
  updated_at: string
}

export interface SavedLogQueryRequest {
  name: string
  app?: string
  spec: LogQuerySpec
  visibility?: SavedLogQueryVisibility
  pinned?: boolean
}
