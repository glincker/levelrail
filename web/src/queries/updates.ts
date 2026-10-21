// Query-key factory and fetcher for GET /api/v1/updates
// (internal/api/updates.go's handleGetUpdates), the Settings > Updates
// page's own data source: running version vs. GitHub's latest published
// release.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// 'edge_image' is the edge channel's pseudo-release: a commit on main
// shipped as a container image, never a GitHub release.
export type ReleaseKind = 'release' | 'edge_image'

export interface UpdateStatus {
  current_version: string
  latest_version: string | null
  update_available: boolean
  release_url: string | null
  published_at: string | null
  release_kind?: ReleaseKind
  channel?: UpdateChannel
  image?: string | null
}

// Mirrors internal/api's edgeGuidance.
export interface EdgeGuidance {
  commit?: string
  commit_url?: string
  image: string
  runs_in_container: boolean
  installable: boolean
  pull_command?: string
  recreate_command?: string
  compose_snippet?: string
  switch_to?: UpdateChannel[]
}

export const updatesKeys = {
  all: ['updates'] as const,
  status: () => [...updatesKeys.all, 'status'] as const,
}

export async function fetchUpdateStatus(): Promise<UpdateStatus> {
  const res = await fetch('/api/v1/updates')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch updates failed: ${res.status}`),
    )
  }
  return (await res.json()) as UpdateStatus
}

export function updatesQueryOptions() {
  return queryOptions({
    queryKey: updatesKeys.status(),
    queryFn: fetchUpdateStatus,
  })
}

export type UpgradeCheckStatus =
  'ok' | 'warn' | 'fail' | 'unknown' | 'pending' | 'skipped'

export type UpgradeFixAction = 'backup_now' | 'recheck' | 'open_rollback'

export interface UpgradeCheck {
  code: string
  name: string
  status: UpgradeCheckStatus
  message: string
  kind?: string
  retryable?: boolean
  protects?: string
  hint?: string
  fix?: { action: UpgradeFixAction }
}

export interface UpdatePreflight {
  current_version: string
  latest_version: string | null
  update_available: boolean
  release_url: string | null
  release_notes: string
  checks: UpgradeCheck[]
  blocked: boolean
  waiting?: boolean
  release_published_at?: string
  upgrade_command: string
  rollback_command: string
  release_kind?: ReleaseKind
  channel?: UpdateChannel
  edge?: EdgeGuidance
  state?: InstallState
  state_reason?: string
  db_schema?: number | null
  latest_schema?: number | null
  migrations_to_apply?: number | null
  restore_point_first?: boolean
  verify_window_seconds?: number
  rollback_version?: string
}

export type InstallState =
  | 'upgrade_available'
  | 'up_to_date'
  | 'ahead_of_releases'
  | 'blocked_older'
  | 'unknown'

// UpdateChannel mirrors internal/upgrade's Channel constants.
export type UpdateChannel = 'stable' | 'beta' | 'edge'

// UpdateSettings mirrors internal/api's updateSettingsResource
// (GET/PUT /api/v1/updates/settings).
export interface UpdateSettings {
  channel: UpdateChannel
  auto_update_enabled: boolean
}

export function updateSettingsQueryOptions() {
  return queryOptions({
    queryKey: [...updatesKeys.all, 'settings'] as const,
    queryFn: async (): Promise<UpdateSettings> => {
      const res = await fetch('/api/v1/updates/settings')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch update settings failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as UpdateSettings
    },
  })
}

export function useUpdateSettings() {
  return useSuspenseQuery(updateSettingsQueryOptions())
}

async function putUpdateSettings(
  settings: UpdateSettings,
): Promise<UpdateSettings> {
  const res = await fetch('/api/v1/updates/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `update update settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as UpdateSettings
}

export function useSetUpdateSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: putUpdateSettings,
    onSuccess: (updated) => {
      queryClient.setQueryData([...updatesKeys.all, 'settings'], updated)
      void queryClient.invalidateQueries({ queryKey: updatesKeys.status() })
    },
  })
}

export function preflightQueryOptions() {
  return queryOptions({
    queryKey: [...updatesKeys.all, 'preflight'] as const,
    queryFn: async (): Promise<UpdatePreflight> => {
      const res = await fetch('/api/v1/updates/preflight')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `fetch preflight failed: ${res.status}`),
        )
      }
      return (await res.json()) as UpdatePreflight
    },
    staleTime: 60_000,
    retry: false,
  })
}
