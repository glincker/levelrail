// GET/PUT /api/v1/settings/heartbeat (internal/api/settings_heartbeat.go).
// URLs are write-only: the server only ever returns their hosts.

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface HeartbeatStatus {
  configured: boolean
  source?: 'env' | 'dashboard'
  host?: string
  fail_host?: string
  interval_seconds: number
  body_enabled: boolean
  can_store_url: boolean
  last_ping_at?: string
  last_result?: 'ok' | 'fail_reported' | 'unhealthy' | 'error'
  last_error?: string
  agent_silence_host?: string
}

export interface HeartbeatUpdate {
  url?: string
  fail_url?: string
  body_enabled?: boolean
  agent_silence_url?: string
}

export const heartbeatKeys = {
  all: ['settings', 'heartbeat'] as const,
}

export async function fetchHeartbeat(): Promise<HeartbeatStatus> {
  const res = await fetch('/api/v1/settings/heartbeat')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch heartbeat failed: ${res.status}`),
    )
  }
  return (await res.json()) as HeartbeatStatus
}

export function heartbeatQueryOptions() {
  return queryOptions({
    queryKey: heartbeatKeys.all,
    queryFn: fetchHeartbeat,
    refetchInterval: 30_000,
  })
}

export function useUpdateHeartbeat() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: HeartbeatUpdate): Promise<HeartbeatStatus> => {
      const res = await fetch('/api/v1/settings/heartbeat', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `save heartbeat failed: ${res.status}`),
        )
      }
      return (await res.json()) as HeartbeatStatus
    },
    onSuccess: (data) => qc.setQueryData(heartbeatKeys.all, data),
  })
}
