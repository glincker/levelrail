// GET /api/v1/databases/{name}/provision and POST .../provision/retry (internal/api/database_provision.go).

import { useEffect, useState } from 'react'
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
  id:
    | 'recheck_docker'
    | 'retry'
    | 'remove_leftover_volume'
    | 'reuse_existing_volume'
    | 'choose_another_name'
    | 'free_disk'
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
  failure?: {
    step: string
    reason: string
    fix?: ProvisionFix
    fixes?: ProvisionFix[]
    volume?: { name: string; size_bytes?: number }
  }
}

export const provisionKeys = {
  one: (name: string) => ['databases', name, 'provision'] as const,
}

const POLL_MS = 1500

// Live updates over SSE; polling only runs while the stream is not connected.
function useProvisionStream(name: string): boolean {
  const enabled = true
  const qc = useQueryClient()
  const [live, setLive] = useState(false)
  useEffect(() => {
    if (!enabled || typeof EventSource === 'undefined') return
    const es = new EventSource(
      `/api/v1/databases/${encodeURIComponent(name)}/provision/stream`,
    )
    es.onopen = () => {
      setLive(true)
    }
    es.onmessage = (ev: MessageEvent<string>) => {
      try {
        qc.setQueryData(
          provisionKeys.one(name),
          JSON.parse(ev.data) as ProvisionAttempt,
        )
      } catch {
        return
      }
    }
    es.onerror = () => {
      setLive(false)
      es.close()
    }
    return () => {
      es.close()
    }
  }, [name, enabled, qc])
  return live
}

export function useDatabaseProvision(name: string) {
  const live = useProvisionStream(name)
  return useProvisionQuery(name, live)
}

function useProvisionQuery(name: string, live: boolean) {
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
      live || (q.state.data && q.state.data.status === 'ready')
        ? false
        : POLL_MS,
  })
}

export interface PendingSettings {
  version?: string
  resources?: { memory_bytes?: number; nano_cpus?: number }
  backup_target_id?: string
  backup_schedule?: string
  backup_retain?: number
  backup_retain_days?: number
}

async function sendProvisionAction(
  name: string,
  method: 'PUT' | 'POST',
  path: string,
  body: object,
): Promise<void> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/${path}`,
    {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
}

// Updates the stored settings of a database that never got a container.
export function useUpdatePendingSettings(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (settings: PendingSettings) =>
      sendProvisionAction(name, 'PUT', 'pending-settings', settings),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['databases'] })
    },
  })
}

// Deletes the record of a database that never got a container.
export function useDiscardPending(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      sendProvisionAction(name, 'POST', 'provision/discard', { confirm: true }),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['databases'] })
    },
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
