// Fetchers for /api/v1/scheduled-maintenance (internal/api/maintenance_api.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type MaintenanceScope = 'all' | 'project' | 'app'
export type MaintenanceState = 'scheduled' | 'active' | 'ended'

export interface ScheduledMaintenance {
  id: string
  title: string
  message: string
  scope: MaintenanceScope
  target?: string
  starts_at: string
  ends_at: string
  freeze: boolean
  announce_lead_minutes: number
  announced_at?: string
  state: MaintenanceState
  created_by?: string
  created_at: string
}

export interface CreateScheduledMaintenance {
  title: string
  message?: string
  scope: MaintenanceScope
  target?: string
  starts_at: string
  ends_at: string
  freeze: boolean
  announce_lead_minutes: number
}

export const maintenanceKeys = {
  all: ['scheduled-maintenance'] as const,
  list: () => [...maintenanceKeys.all, 'list'] as const,
  banner: () => [...maintenanceKeys.all, 'banner'] as const,
}

const BASE = '/api/v1/scheduled-maintenance'
const BANNER_REFETCH_MS = 60_000

async function readList(url: string, signal?: AbortSignal) {
  const res = await fetch(url, { signal })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `load maintenance failed: ${res.status}`),
    )
  }
  return (await res.json()) as ScheduledMaintenance[]
}

export function useScheduledMaintenance() {
  return useQuery({
    queryKey: maintenanceKeys.list(),
    queryFn: ({ signal }) => readList(BASE, signal),
  })
}

/** Windows the dashboard banner shows: active, or inside their lead time. */
export function useMaintenanceBanner() {
  return useQuery({
    queryKey: maintenanceKeys.banner(),
    queryFn: ({ signal }) => readList(`${BASE}?current=true`, signal),
    refetchInterval: BANNER_REFETCH_MS,
    staleTime: BANNER_REFETCH_MS / 2,
    retry: false,
  })
}

export function useCreateScheduledMaintenance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (
      body: CreateScheduledMaintenance,
    ): Promise<ScheduledMaintenance> => {
      const res = await fetch(BASE, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `schedule maintenance failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as ScheduledMaintenance
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: maintenanceKeys.all }),
  })
}

export function useDeleteScheduledMaintenance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string): Promise<void> => {
      const res = await fetch(`${BASE}/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `remove maintenance failed: ${res.status}`,
          ),
        )
      }
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: maintenanceKeys.all }),
  })
}
