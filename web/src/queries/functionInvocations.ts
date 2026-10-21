// GET /api/v1/functions/{name}/invocations, POST .../invoke, POST .../invocations/{id}/replay and
// GET/PUT .../capture (internal/api/functions_invoke.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface FunctionInvocation {
  id: string
  time: string
  method: string
  path: string
  status: number
  duration_ms: number
  cold_start: boolean
  bytes_in: number
  bytes_out: number
  trace_id?: string
  replayable: boolean
  logs_from: string
  logs_to: string
}

export interface FunctionInvocations {
  invocations: FunctionInvocation[]
  capture: boolean
}

export interface InvokeInput {
  method: string
  path: string
  headers: Record<string, string>
  body: string
}

export interface InvokeResult {
  request_id: string
  status: number
  headers: Record<string, string[]>
  body: string
  body_truncated?: boolean
  duration_ms: number
  cold_start: boolean
  input_stored: boolean
}

const base = (name: string) => `/api/v1/functions/${encodeURIComponent(name)}`

export const invocationKeys = {
  list: (name: string) => ['functions', 'invocations', name] as const,
}

async function send<T>(
  path: string,
  what: string,
  method: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useFunctionInvocations(name: string) {
  return useQuery({
    queryKey: invocationKeys.list(name),
    queryFn: () =>
      send<FunctionInvocations>(
        `${base(name)}/invocations`,
        'fetch invocations',
        'GET',
      ),
    refetchInterval: 10_000,
  })
}

export function useInvokeFunction(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: InvokeInput) =>
      send<InvokeResult>(
        `${base(name)}/invoke`,
        'invoke function',
        'POST',
        input,
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: invocationKeys.list(name) }),
  })
}

export function useReplayInvocation(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      send<InvokeResult>(
        `${base(name)}/invocations/${encodeURIComponent(id)}/replay`,
        'replay invocation',
        'POST',
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: invocationKeys.list(name) }),
  })
}

export function useSetCapture(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (enabled: boolean) =>
      send<{ enabled: boolean }>(
        `${base(name)}/capture`,
        'update capture',
        'PUT',
        {
          enabled,
        },
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: invocationKeys.list(name) }),
  })
}
