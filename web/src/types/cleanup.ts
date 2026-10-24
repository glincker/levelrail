// Matches the cleanup center API (internal/cleanup, /api/v1/cleanup).

export type CleanupCategory =
  | 'auth'
  | 'audit'
  | 'deploys'
  | 'docker'
  | 'data'
  | 'observability'
  | 'messaging'
  | 'other'

export type CleanupMode = 'off' | 'preview' | 'on'
export type CleanupSource = 'default' | 'env' | 'saved'
export type CleanupOrigin = 'new' | 'existing'
export type CleanupTrigger = 'scheduled' | 'manual' | 'cli' | 'api'

export interface CleanupUsage {
  rows: number
  /** Null when the store cannot measure size. */
  bytes: number | null
  oldest?: string | null
}

export type CleanupLastRun = CleanupRun

export interface CleanupTarget {
  id: string
  category: CleanupCategory
  title: string
  description: string
  /** Runtime setting key holding the retention preset; empty for read-through targets. */
  policy_key: string
  days: number
  /** Where the retention days came from. */
  source?: CleanupSource
  mode: CleanupMode
  origin: CleanupOrigin
  destructive: boolean
  floor: number
  usage: CleanupUsage
  last_run?: CleanupLastRun | null
  next_run?: string | null
  read_through?: boolean
}

export interface CleanupScheduler {
  enabled: boolean
  interval: string
}

export interface CleanupOverview {
  targets: CleanupTarget[]
  scheduler: CleanupScheduler
}

export interface CleanupImpact {
  rows: number
  bytes: number
}

export interface CleanupResult {
  rows: number
  bytes: number
  dry_run?: boolean
  error?: string
  more_remaining?: boolean
}

export interface CleanupRun {
  id: number
  target: string
  started_at: string
  finished_at?: string | null
  days: number
  dry_run: boolean
  rows: number
  bytes: number
  trigger: CleanupTrigger
  actor?: string
  error?: string
}

export type CleanupRunStatus = 'all' | 'ok' | 'failed'
export type CleanupRunKind = 'all' | 'dry' | 'real'

export interface CleanupRunFilters {
  target: string
  since: string
  until: string
  kind: CleanupRunKind
  status: CleanupRunStatus
}

export interface CleanupRunsPage {
  runs: CleanupRun[]
  next_cursor?: string
}
