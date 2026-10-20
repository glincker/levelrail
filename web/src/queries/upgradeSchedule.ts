// Scheduled upgrades, the automatic upgrade policy and the preflight fix
// actions behind Settings > Updates (internal/api/updates_schedule.go).

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { updatesKeys, type UpdatePreflight } from './updates'

export type ScheduleState =
  | 'pending'
  | 'blocked'
  | 'running'
  | 'applied'
  | 'failed'
  | 'cancelled'
  | 'missed'

export interface UpgradeSchedule {
  id: string
  target_version: string
  source: 'manual' | 'auto'
  mode: 'at' | 'window'
  state: ScheduleState
  run_at: string
  retry_until: string
  timezone: string
  acked: string[]
  major_acked: boolean
  created_by: string
  created_at: string
  check_count: number
  last_check_at?: string
  last_reason?: string
  fired_at?: string
  finished_at?: string
}

export interface UpgradeWindow {
  start: string
  end: string
  source: string
  active: boolean
}

export interface UpgradeScheduleStatus {
  available: boolean
  server_timezone: string
  now: string
  active: UpgradeSchedule | null
  recent: UpgradeSchedule[]
  next_window: UpgradeWindow | null
}

export interface ScheduleRequest {
  target?: string
  at_local?: string
  timezone?: string
  window?: boolean
  ack?: string[]
  acknowledge_major?: boolean
  replace?: boolean
}

export type PolicyMode = 'off' | 'patch' | 'any'

export interface UpgradePolicy {
  mode: PolicyMode
  effective: PolicyMode | 'paused'
  window_cron: string
  window_duration_minutes: number
  timezone: string
  min_release_age_days: number
  notify_channel_ids: string[]
  paused: boolean
  updated_by?: string
  updated_at?: string
  decision?: string
  decision_tag?: string
  decision_at?: string
  server_timezone: string
  next_window: UpgradeWindow | null
}

export type UpgradePolicyInput = Pick<
  UpgradePolicy,
  | 'mode'
  | 'window_cron'
  | 'window_duration_minutes'
  | 'timezone'
  | 'min_release_age_days'
  | 'notify_channel_ids'
  | 'paused'
>

export const scheduleKeys = {
  schedule: () => [...updatesKeys.all, 'schedule'] as const,
  policy: () => [...updatesKeys.all, 'auto'] as const,
  preflight: () => [...updatesKeys.all, 'preflight'] as const,
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function scheduleQueryOptions() {
  return queryOptions({
    queryKey: scheduleKeys.schedule(),
    queryFn: async (): Promise<UpgradeScheduleStatus> => {
      const res = await fetch('/api/v1/updates/schedule')
      if (!res.ok) throw await failure(res, 'fetch upgrade schedule')
      return (await res.json()) as UpgradeScheduleStatus
    },
    staleTime: 15_000,
    retry: false,
  })
}

export function policyQueryOptions() {
  return queryOptions({
    queryKey: scheduleKeys.policy(),
    queryFn: async (): Promise<UpgradePolicy> => {
      const res = await fetch('/api/v1/updates/auto')
      if (!res.ok) throw await failure(res, 'fetch automatic upgrade policy')
      return (await res.json()) as UpgradePolicy
    },
    staleTime: 30_000,
    retry: false,
  })
}

export function useSetPolicy() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (input: UpgradePolicyInput): Promise<UpgradePolicy> => {
      const res = await fetch('/api/v1/updates/auto', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) throw await failure(res, 'save automatic upgrades')
      return (await res.json()) as UpgradePolicy
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(scheduleKeys.policy(), updated)
      void queryClient.invalidateQueries({ queryKey: scheduleKeys.schedule() })
    },
  })
}

export class ScheduleError extends ApiError {
  readonly missingAck: string[]
  readonly needsMajorAck: boolean
  constructor(
    status: number,
    message: string,
    missingAck: string[],
    needsMajorAck: boolean,
  ) {
    super(status, message)
    this.missingAck = missingAck
    this.needsMajorAck = needsMajorAck
  }
}

export function useCreateSchedule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (input: ScheduleRequest): Promise<UpgradeSchedule> => {
      const res = await fetch('/api/v1/updates/schedule', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) {
        let missing: string[] = []
        let major = false
        try {
          const body = (await res.clone().json()) as {
            missing_ack?: string[]
            needs_major_ack?: boolean
          }
          missing = body.missing_ack ?? []
          major = body.needs_major_ack === true
        } catch {
          // The error body is not JSON: the generic message below is enough.
        }
        throw new ScheduleError(
          res.status,
          await readErrorMessage(res, `schedule upgrade failed: ${res.status}`),
          missing,
          major,
        )
      }
      return (await res.json()) as UpgradeSchedule
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: scheduleKeys.schedule() }),
  })
}

export function useCancelSchedule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (): Promise<UpgradeSchedule> => {
      const res = await fetch('/api/v1/updates/schedule', { method: 'DELETE' })
      if (!res.ok) throw await failure(res, 'cancel scheduled upgrade')
      return (await res.json()) as UpgradeSchedule
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: scheduleKeys.schedule() }),
  })
}

interface BackupNowResult {
  backup: { name: string; size_bytes: number; created_at: string }
  preflight: UpdatePreflight
}

// useBackupNow takes a control plane backup and writes the preflight the
// server evaluated right after it, so the stale-backup blocker clears with
// no second round trip.
export function useBackupNow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (): Promise<BackupNowResult> => {
      const res = await fetch('/api/v1/updates/backup-now', { method: 'POST' })
      if (!res.ok) throw await failure(res, 'back up now')
      return (await res.json()) as BackupNowResult
    },
    onSuccess: (result) => {
      queryClient.setQueryData(scheduleKeys.preflight(), result.preflight)
    },
  })
}

// useRecheckPreflight re-runs the checks bypassing the release cache, which
// is what clears a release that was still being published a moment ago.
export function useRecheckPreflight() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (): Promise<UpdatePreflight> => {
      const res = await fetch('/api/v1/updates/preflight?refresh=1')
      if (!res.ok) throw await failure(res, 're-check upgrade preflight')
      return (await res.json()) as UpdatePreflight
    },
    onSuccess: (result) => {
      queryClient.setQueryData(scheduleKeys.preflight(), result)
      void queryClient.invalidateQueries({ queryKey: updatesKeys.status() })
    },
  })
}
