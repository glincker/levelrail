// Wire types for the cross-app pipeline reads in
// internal/api/pipelines_overview.go (snake_case JSON).

import type { PipelineStatus, PipelineTrigger } from './pipelines'

export interface PipelineRunRow {
  id: string
  app: string
  pipeline: string
  number: number
  status: PipelineStatus
  reason?: string
  trigger: PipelineTrigger
  ref?: string
  short_sha?: string
  created_at: string
  started_at?: string
  duration_seconds?: number
  approval_pending: boolean
  approval_id?: number
  hold_pending: boolean
  can_decide: boolean
}

export interface PipelineRunRowsPage {
  runs: PipelineRunRow[]
  next_cursor?: string
}

export interface PipelineSummary {
  running: number
  succeeded_24h: number
  failed_24h: number
  cancelled_24h: number
  waiting_approval: number
  held: number
  success_rate_24h: number | null
}

export type PipelineOverviewStatusFilter =
  'running' | 'failed' | 'succeeded' | 'cancelled' | 'waiting_approval' | 'held'

export interface PipelineOverviewFilters {
  status?: PipelineOverviewStatusFilter
  app?: string
  pipeline?: string
  trigger?: PipelineTrigger
}
