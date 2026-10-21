// Function triggers and the async invocation queue (internal/api/function_triggers.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface FunctionWebhook {
  url: string
  path: string
  replay_window_seconds: number
  created_at: string
  rotated_at?: string
  secret?: string
}

export interface FunctionSchedule {
  id: string
  name: string
  schedule: string
  timezone?: string
  enabled: boolean
}

export interface FunctionTriggers {
  function: string
  webhook: FunctionWebhook | null
  schedules: FunctionSchedule[]
  queue: Record<string, number>
}

export interface FunctionInvocation {
  id: string
  function: string
  source: string
  method: string
  path: string
  body_bytes: number
  status: string
  attempts: number
  max_attempts: number
  next_attempt_at: string
  last_status_code?: number
  last_error?: string
  created_at: string
}

export const triggerKeys = {
  all: ['function-triggers'] as const,
  one: (name: string) => ['function-triggers', 'one', name] as const,
  dead: ['function-triggers', 'dead'] as const,
}

async function request<T>(
  path: string,
  what: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function functionTriggersQueryOptions(name: string) {
  return queryOptions({
    queryKey: triggerKeys.one(name),
    queryFn: () =>
      request<FunctionTriggers>(
        `/api/v1/functions/${encodeURIComponent(name)}/triggers`,
        'fetch function triggers',
      ),
    refetchInterval: 15_000,
  })
}

export function useFunctionTriggers(name: string) {
  return useQuery({ ...functionTriggersQueryOptions(name), enabled: !!name })
}

export function useDeadLetters() {
  return useQuery({
    queryKey: triggerKeys.dead,
    queryFn: async () =>
      (
        await request<{ invocations: FunctionInvocation[] }>(
          '/api/v1/function-invocations?status=dead',
          'fetch dead letters',
        )
      ).invocations,
    refetchInterval: 15_000,
  })
}

export function useSetWebhook(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { path: string; rotate: boolean }) =>
      request<FunctionWebhook>(
        `/api/v1/functions/${encodeURIComponent(name)}/triggers/webhook`,
        'set webhook',
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: triggerKeys.all }),
  })
}

export function useDisableWebhook(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      request<void>(
        `/api/v1/functions/${encodeURIComponent(name)}/triggers/webhook`,
        'disable webhook',
        { method: 'DELETE' },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: triggerKeys.all }),
  })
}

export function useRetryInvocation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `/api/v1/function-invocations/${encodeURIComponent(id)}/retry`,
        'retry invocation',
        { method: 'POST' },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: triggerKeys.all }),
  })
}

export function useDiscardInvocation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `/api/v1/function-invocations/${encodeURIComponent(id)}`,
        'discard invocation',
        { method: 'DELETE' },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: triggerKeys.all }),
  })
}
