// Query keys, fetchers and mutations for /api/v1/settings/traffic-export
// (internal/api/traffic_export.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  TrafficExportSink,
  TrafficExportSinkInput,
  TrafficExportSinkList,
  TrafficExportStatus,
  TrafficExportTestResult,
} from '../types/trafficExport'

export const trafficExportKeys = {
  all: ['traffic-export'] as const,
  sinks: () => [...trafficExportKeys.all, 'sinks'] as const,
  status: () => [...trafficExportKeys.all, 'status'] as const,
}

// Queue depth and counters move every few seconds, so status is polled.
const STATUS_REFETCH_MS = 10_000

const base = '/api/v1/settings/traffic-export'

async function request<T>(
  path: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  }
}

export function useTrafficExportSinks() {
  return useQuery(
    queryOptions({
      queryKey: trafficExportKeys.sinks(),
      queryFn: () =>
        request<TrafficExportSinkList>(
          `${base}/sinks`,
          undefined,
          'fetch traffic export sinks',
        ),
      retry: false,
    }),
  )
}

export function useTrafficExportStatus() {
  return useQuery(
    queryOptions({
      queryKey: trafficExportKeys.status(),
      queryFn: () =>
        request<TrafficExportStatus>(
          `${base}/status`,
          undefined,
          'fetch traffic export status',
        ),
      refetchInterval: STATUS_REFETCH_MS,
      retry: false,
    }),
  )
}

function useInvalidatingMutation<TVars, TData>(
  fn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient()
  return useMutation<TData, ApiError, TVars>({
    mutationFn: fn,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: trafficExportKeys.all }),
  })
}

export function useCreateTrafficExportSink() {
  return useInvalidatingMutation((body: TrafficExportSinkInput) =>
    request<TrafficExportSink>(
      `${base}/sinks`,
      jsonInit('POST', body),
      'create traffic export sink',
    ),
  )
}

export function useUpdateTrafficExportSink() {
  return useInvalidatingMutation(
    (vars: { id: string; body: Partial<TrafficExportSinkInput> }) =>
      request<TrafficExportSink>(
        `${base}/sinks/${encodeURIComponent(vars.id)}`,
        jsonInit('PUT', vars.body),
        'update traffic export sink',
      ),
  )
}

export function useDeleteTrafficExportSink() {
  return useInvalidatingMutation((id: string) =>
    request<void>(
      `${base}/sinks/${encodeURIComponent(id)}`,
      { method: 'DELETE' },
      'delete traffic export sink',
    ),
  )
}

export function useTestTrafficExportSink() {
  return useInvalidatingMutation((id: string) =>
    request<TrafficExportTestResult>(
      `${base}/sinks/${encodeURIComponent(id)}/test`,
      { method: 'POST' },
      'test traffic export sink',
    ),
  )
}
