// Data sources for the Settings > Updates release history and rollback
// preview (internal/api/updates_releases.go). Both are read-only.

import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { updatesKeys, type UpgradeCheck } from './updates'

export type HistoryView = 'stable' | 'beta' | 'all'

export type SchemaVerdict =
  'binary_only' | 'forward' | 'restore_required' | 'unknown'

export interface ReleaseHistoryItem {
  version: string
  url: string
  published_at: string
  channel: 'stable' | 'beta'
  running: boolean
  retained: boolean
  asset_name: string
  asset_available: boolean
  asset_size: number
  signed: boolean
  schema_version: number | null
  schema_source: 'retained' | 'manifest' | 'unknown'
  verdict: SchemaVerdict
  notes: string
}

export interface ReleaseHistory {
  current_version: string
  current_schema_version: number | null
  channel: string
  view: HistoryView
  github_reachable: boolean
  releases: ReleaseHistoryItem[]
  retained_only: ReleaseHistoryItem[]
}

export interface RollbackBackup {
  name: string
  created_at: string
  schema_version: number
  compatible: boolean
}

export interface RollbackChange {
  version: string
  notes: string
}

export interface RollbackPlan {
  current_version: string
  current_schema_version: number
  target: {
    version: string
    schema_version: number
    schema_source: string
    retained: boolean
  }
  verdict: SchemaVerdict
  steps: string[]
  warnings: string[]
  downtime_seconds: number
  restore_required: boolean
  backups: RollbackBackup[]
  data_loss_since?: string
  command: string
  restore_command?: string
  fetch_command?: string
  checks: UpgradeCheck[]
  blocked: boolean
  changes: RollbackChange[]
  notes: string
}

async function getJSON<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function releaseHistoryQueryOptions(view: HistoryView | null) {
  return queryOptions({
    queryKey: [...updatesKeys.all, 'releases', view ?? 'default'] as const,
    queryFn: () =>
      getJSON<ReleaseHistory>(
        view
          ? `/api/v1/updates/releases?channel=${view}`
          : '/api/v1/updates/releases',
        'fetch release history',
      ),
    staleTime: 60_000,
  })
}

export function rollbackPlanQueryOptions(version: string | null) {
  return queryOptions({
    queryKey: [...updatesKeys.all, 'rollback-plan', version] as const,
    queryFn: () =>
      getJSON<RollbackPlan>(
        `/api/v1/updates/rollback-plan?version=${encodeURIComponent(version ?? '')}`,
        'fetch rollback plan',
      ),
    enabled: version !== null,
  })
}

export interface DoctorCheck {
  code: string
  name: string
  status: 'ok' | 'warn' | 'fail'
  message: string
}

export interface DoctorReport {
  ok: boolean
  checks: DoctorCheck[]
}

export function doctorQueryOptions() {
  return queryOptions({
    queryKey: ['system', 'doctor', 'post-upgrade'] as const,
    queryFn: () =>
      getJSON<DoctorReport>('/api/v1/system/doctor', 'run health verification'),
    enabled: false,
    staleTime: 0,
  })
}
