// GET /api/v1/databases/{name}/provision and POST .../provision/retry (internal/api/database_provision.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type ProvisionStatus =
  'pending' | 'started' | 'done' | 'failed' | 'skipped'

export interface ProvisionStep {
  step: string
  status: ProvisionStatus
  detail?: string
  current?: number
  total?: number
  at?: string
}

export interface ProvisionFix {
  id: 'recheck_docker' | 'retry' | 'remove_leftover_volume' | 'free_disk'
  label: string
  confirm?: boolean
}

export interface ProvisionAttempt {
  attempt_id: string
  status: 'in_progress' | 'failed' | 'ready'
  current_step?: string
  started_at?: string
  finished_at?: string
  steps: ProvisionStep[]
  events: {
    step: string
    status: ProvisionStatus
    detail?: string
    at: string
  }[]
  failure?: { step: string; reason: string; fix?: ProvisionFix }
}

export const provisionKeys = {
  one: (name: string) => ['databases', name, 'provision'] as const,
}

const POLL_MS = 1500

export function useDatabaseProvision(name: string) {
  return useQuery({
    queryKey: provisionKeys.one(name),
    queryFn: async (): Promise<ProvisionAttempt | null> => {
      const res = await fetch(
        `/api/v1/databases/${encodeURIComponent(name)}/provision`,
      )
      if (res.status === 404) return null
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `provision failed: ${res.status}`),
        )
      }
      return (await res.json()) as ProvisionAttempt
    },
    retry: false,
    refetchInterval: (q) =>
      q.state.data && q.state.data.status === 'ready' ? false : POLL_MS,
  })
}

export function useRetryProvision(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (vars: { fix?: string; confirm?: boolean }) => {
      const res = await fetch(
        `/api/v1/databases/${encodeURIComponent(name)}/provision/retry`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(vars),
        },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `retry failed: ${res.status}`),
        )
      }
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: provisionKeys.one(name) })
    },
  })
}
