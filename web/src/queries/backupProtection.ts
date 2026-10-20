// Query-key factory, fetchers and mutations for backup health, restore
// drills, bucket protection and volume backup policy
// (internal/api/backup_protection.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  BackupDrill,
  BackupHealth,
  TargetProtection,
  VolumeBackupPolicy,
  VolumeRestoreToRequest,
  VolumeRestoreToResponse,
} from '../types/backupProtection'

export const backupProtectionKeys = {
  all: ['backup-protection'] as const,
  health: () => [...backupProtectionKeys.all, 'health'] as const,
  drills: (filter: string) =>
    [...backupProtectionKeys.all, 'drills', filter] as const,
  policy: (app: string, volume: string) =>
    [...backupProtectionKeys.all, 'policy', app, volume] as const,
}

const RUNNING_POLL_INTERVAL_MS = 3_000

async function failure(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export async function fetchBackupHealth(): Promise<BackupHealth> {
  const res = await fetch('/api/v1/backups/health')
  if (!res.ok) return failure(res, 'fetch backup health')
  const body = (await res.json()) as BackupHealth
  return {
    ...body,
    resources: body.resources ?? [],
    targets: body.targets ?? [],
  }
}

export function backupHealthQueryOptions() {
  return queryOptions({
    queryKey: backupProtectionKeys.health(),
    queryFn: fetchBackupHealth,
  })
}

export function useBackupHealth() {
  return useQuery(backupHealthQueryOptions())
}

export interface DrillFilter {
  service?: string
  volume?: string
  database?: string
  limit?: number
}

export async function fetchBackupDrills(
  filter: DrillFilter,
): Promise<BackupDrill[]> {
  const params = new URLSearchParams()
  if (filter.service) params.set('service', filter.service)
  if (filter.volume) params.set('volume', filter.volume)
  if (filter.database) params.set('database', filter.database)
  if (filter.limit) params.set('limit', String(filter.limit))
  const query = params.toString()
  const res = await fetch(`/api/v1/backups/drills${query ? `?${query}` : ''}`)
  if (!res.ok) return failure(res, 'fetch restore drills')
  return ((await res.json()) as BackupDrill[] | null) ?? []
}

export function useBackupDrills(filter: DrillFilter = {}) {
  return useQuery({
    queryKey: backupProtectionKeys.drills(JSON.stringify(filter)),
    queryFn: () => fetchBackupDrills(filter),
    refetchInterval: (query) =>
      query.state.data?.some((d) => d.status === 'running')
        ? RUNNING_POLL_INTERVAL_MS
        : false,
  })
}

export function useStartBackupDrill() {
  const queryClient = useQueryClient()
  return useMutation<{ id: string }, ApiError, string>({
    mutationFn: async (backupId) => {
      const res = await fetch('/api/v1/backups/drills', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ backup_id: backupId }),
      })
      if (!res.ok) return failure(res, 'start restore drill')
      return (await res.json()) as { id: string }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: backupProtectionKeys.all })
    },
  })
}

export function useRefreshBackupProtection() {
  const queryClient = useQueryClient()
  return useMutation<TargetProtection[], ApiError, void>({
    mutationFn: async () => {
      const res = await fetch('/api/v1/backups/protection/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      })
      if (!res.ok) return failure(res, 'probe backup buckets')
      return (await res.json()) as TargetProtection[]
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: backupProtectionKeys.health(),
      })
    },
  })
}

function policyPath(app: string, volume: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/volumes/${encodeURIComponent(volume)}/backup-policy`
}

export function volumeBackupPolicyQueryOptions(app: string, volume: string) {
  return queryOptions({
    queryKey: backupProtectionKeys.policy(app, volume),
    queryFn: async (): Promise<VolumeBackupPolicy> => {
      const res = await fetch(policyPath(app, volume))
      if (!res.ok) return failure(res, 'fetch volume backup policy')
      return (await res.json()) as VolumeBackupPolicy
    },
  })
}

export function useVolumeBackupPolicy(
  app: string,
  volume: string,
  enabled: boolean,
) {
  return useQuery({ ...volumeBackupPolicyQueryOptions(app, volume), enabled })
}

export function useSetVolumeBackupPolicy(app: string, volume: string) {
  const queryClient = useQueryClient()
  return useMutation<VolumeBackupPolicy, ApiError, VolumeBackupPolicy>({
    mutationFn: async (policy) => {
      const res = await fetch(policyPath(app, volume), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(policy),
      })
      if (!res.ok) return failure(res, 'save volume backup policy')
      return (await res.json()) as VolumeBackupPolicy
    },
    onSuccess: (saved) => {
      queryClient.setQueryData(backupProtectionKeys.policy(app, volume), saved)
    },
  })
}

export function useRestoreVolumeTo(app: string, volume: string) {
  const queryClient = useQueryClient()
  return useMutation<VolumeRestoreToResponse, ApiError, VolumeRestoreToRequest>(
    {
      mutationFn: async (req) => {
        const res = await fetch(
          `/api/v1/apps/${encodeURIComponent(app)}/volumes/${encodeURIComponent(volume)}/restore-to`,
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(req),
          },
        )
        if (!res.ok) return failure(res, 'restore volume')
        return (await res.json()) as VolumeRestoreToResponse
      },
      onSuccess: () => {
        void queryClient.invalidateQueries({
          queryKey: ['apps', app, 'volumes', volume, 'clone-restores'],
        })
      },
    },
  )
}
