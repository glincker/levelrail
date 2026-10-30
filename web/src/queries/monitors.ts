import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  CreateMonitorInput,
  Monitor,
  MonitorResults,
} from '../types/monitor'

export const monitorKeys = {
  all: ['monitors'] as const,
  list: () => [...monitorKeys.all, 'list'] as const,
  results: (id: string) => [...monitorKeys.all, 'results', id] as const,
}

// 404/501 mean monitors are off; null keeps the section hidden.
export async function fetchMonitors(): Promise<Monitor[] | null> {
  const res = await fetch('/api/v1/monitors')
  if (res.status === 404 || res.status === 501) return null
  if (!res.ok) throw new Error(`fetch monitors failed: ${res.status}`)
  return (await res.json()) as Monitor[]
}

export async function fetchMonitorResults(id: string): Promise<MonitorResults> {
  const res = await fetch(
    `/api/v1/monitors/${encodeURIComponent(id)}/results?window=24h&limit=50`,
  )
  if (!res.ok) throw new Error(`fetch monitor results failed: ${res.status}`)
  return (await res.json()) as MonitorResults
}

export function useMonitors() {
  return useQuery({
    queryKey: monitorKeys.list(),
    queryFn: fetchMonitors,
    retry: false,
  })
}

export function useMonitorResults(id: string) {
  return useQuery({
    queryKey: monitorKeys.results(id),
    queryFn: () => fetchMonitorResults(id),
    retry: false,
    refetchInterval: 60_000,
  })
}

export function useCreateMonitor() {
  const queryClient = useQueryClient()
  return useMutation<Monitor, ApiError, CreateMonitorInput>({
    mutationFn: async (input) => {
      const res = await fetch('/api/v1/monitors', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `create monitor failed: ${res.status}`),
        )
      }
      return (await res.json()) as Monitor
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: monitorKeys.all })
    },
  })
}

export function useDeleteMonitor() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (id) => {
      const res = await fetch(`/api/v1/monitors/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `delete monitor failed: ${res.status}`),
        )
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: monitorKeys.all })
    },
  })
}
